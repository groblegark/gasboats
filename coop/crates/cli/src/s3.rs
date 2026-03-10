// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Alfred Jean LLC

//! S3 persistence for session data (transcripts, recordings, event logs).
//!
//! When configured via `COOP_S3_BUCKET`, subscribes to broadcast channels
//! and uploads session artifacts to S3 in real time. Supports downloading
//! artifacts from S3 for session resume.
//!
//! The [`S3Storage`] trait abstracts storage operations so that tests can
//! use an in-memory mock without requiring a real S3 endpoint.

use std::future::Future;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tokio::sync::broadcast;
use tokio_util::sync::CancellationToken;
use tracing::{info, warn};

use crate::record::RecordingEntry;
use crate::transcript::TranscriptEvent;

/// Trait abstracting S3 storage operations for testability.
///
/// [`S3Client`] implements this for real AWS S3. Tests can supply a mock.
pub trait S3Storage: Send + Sync + 'static {
    fn upload_file(
        &self,
        session_id: &str,
        s3_path: &str,
        local_path: &Path,
    ) -> impl Future<Output = anyhow::Result<()>> + Send;

    fn upload_bytes(
        &self,
        session_id: &str,
        s3_path: &str,
        data: Vec<u8>,
    ) -> impl Future<Output = anyhow::Result<()>> + Send;

    fn download_file(
        &self,
        session_id: &str,
        s3_path: &str,
        local_path: &Path,
    ) -> impl Future<Output = anyhow::Result<()>> + Send;

    fn list_transcripts(
        &self,
        session_id: &str,
    ) -> impl Future<Output = anyhow::Result<Vec<u32>>> + Send;

    fn upload_meta(
        &self,
        session_id: &str,
        meta: &SessionMeta,
    ) -> impl Future<Output = anyhow::Result<()>> + Send;

    fn download_meta(
        &self,
        session_id: &str,
    ) -> impl Future<Output = anyhow::Result<Option<SessionMeta>>> + Send;

    fn exists(&self, session_id: &str, s3_path: &str) -> impl Future<Output = bool> + Send;
}

/// S3 client wrapper for session persistence.
pub struct S3Client {
    client: aws_sdk_s3::Client,
    bucket: String,
    prefix: String,
}

/// Session metadata stored alongside artifacts in S3.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionMeta {
    pub session_id: String,
    pub agent_type: String,
    pub started_at: u64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ended_at: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub exit_code: Option<i32>,
    #[serde(default)]
    pub labels: Vec<String>,
}

fn now_unix_secs() -> u64 {
    std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).unwrap_or_default().as_secs()
}

impl S3Client {
    /// Create a new S3 client from environment/IRSA credentials.
    pub async fn new(
        bucket: String,
        prefix: String,
        region: Option<String>,
    ) -> anyhow::Result<Self> {
        let mut config_loader = aws_config::defaults(aws_config::BehaviorVersion::latest());
        if let Some(ref region) = region {
            config_loader = config_loader.region(aws_config::Region::new(region.clone()));
        }
        let sdk_config = config_loader.load().await;
        let client = aws_sdk_s3::Client::new(&sdk_config);

        // Verify connectivity with a lightweight HEAD request.
        match client.head_bucket().bucket(&bucket).send().await {
            Ok(_) => info!(bucket = %bucket, prefix = %prefix, "s3: connected"),
            Err(e) => {
                warn!(bucket = %bucket, error = %e, "s3: bucket HEAD failed (may still work)");
            }
        }

        Ok(Self { client, bucket, prefix })
    }

    /// S3 key for a session artifact.
    fn key(&self, session_id: &str, path: &str) -> String {
        format!("{}/{}/{}", self.prefix, session_id, path)
    }
}

impl S3Storage for S3Client {
    async fn upload_file(
        &self,
        session_id: &str,
        s3_path: &str,
        local_path: &Path,
    ) -> anyhow::Result<()> {
        let key = self.key(session_id, s3_path);
        let body = aws_sdk_s3::primitives::ByteStream::from_path(local_path).await?;
        self.client.put_object().bucket(&self.bucket).key(&key).body(body).send().await?;
        Ok(())
    }

    async fn upload_bytes(
        &self,
        session_id: &str,
        s3_path: &str,
        data: Vec<u8>,
    ) -> anyhow::Result<()> {
        let key = self.key(session_id, s3_path);
        let body = aws_sdk_s3::primitives::ByteStream::from(data);
        self.client.put_object().bucket(&self.bucket).key(&key).body(body).send().await?;
        Ok(())
    }

    async fn download_file(
        &self,
        session_id: &str,
        s3_path: &str,
        local_path: &Path,
    ) -> anyhow::Result<()> {
        let key = self.key(session_id, s3_path);
        let output = self.client.get_object().bucket(&self.bucket).key(&key).send().await?;
        let bytes = output.body.collect().await?.into_bytes();
        if let Some(parent) = local_path.parent() {
            tokio::fs::create_dir_all(parent).await?;
        }
        tokio::fs::write(local_path, bytes).await?;
        Ok(())
    }

    async fn list_transcripts(&self, session_id: &str) -> anyhow::Result<Vec<u32>> {
        let prefix = self.key(session_id, "transcripts/");
        let mut numbers = Vec::new();
        let mut continuation = None;

        loop {
            let mut req = self.client.list_objects_v2().bucket(&self.bucket).prefix(&prefix);
            if let Some(token) = continuation {
                req = req.continuation_token(token);
            }
            let output = req.send().await?;

            if let Some(contents) = output.contents() {
                for obj in contents {
                    if let Some(key) = obj.key() {
                        // Extract number from "prefix/session-id/transcripts/N.jsonl"
                        if let Some(filename) = key.rsplit('/').next() {
                            if let Some(num_str) = filename.strip_suffix(".jsonl") {
                                if let Ok(n) = num_str.parse::<u32>() {
                                    numbers.push(n);
                                }
                            }
                        }
                    }
                }
            }

            if output.is_truncated() == Some(true) {
                continuation = output.next_continuation_token().map(|s| s.to_owned());
            } else {
                break;
            }
        }

        numbers.sort();
        Ok(numbers)
    }

    async fn upload_meta(&self, session_id: &str, meta: &SessionMeta) -> anyhow::Result<()> {
        let json = serde_json::to_vec_pretty(meta)?;
        self.upload_bytes(session_id, "meta.json", json).await
    }

    async fn download_meta(&self, session_id: &str) -> anyhow::Result<Option<SessionMeta>> {
        let key = self.key(session_id, "meta.json");
        match self.client.get_object().bucket(&self.bucket).key(&key).send().await {
            Ok(output) => {
                let bytes = output.body.collect().await?.into_bytes();
                let meta: SessionMeta = serde_json::from_slice(&bytes)?;
                Ok(Some(meta))
            }
            Err(_) => Ok(None),
        }
    }

    async fn exists(&self, session_id: &str, s3_path: &str) -> bool {
        let key = self.key(session_id, s3_path);
        self.client.head_object().bucket(&self.bucket).key(&key).send().await.is_ok()
    }
}

/// Spawn the S3 persistence subscriber.
///
/// Listens to transcript and recording broadcast channels and uploads
/// artifacts to S3 as they are created. Also periodically uploads the
/// session log for crash resilience.
pub fn spawn_subscriber<S: S3Storage>(
    s3: Arc<S>,
    session_id: String,
    session_dir: Option<PathBuf>,
    transcript_tx: &broadcast::Sender<TranscriptEvent>,
    record_tx: &broadcast::Sender<RecordingEntry>,
    session_log_path: Option<PathBuf>,
    upload_interval: Duration,
    agent_type: String,
    labels: Vec<String>,
    shutdown: CancellationToken,
) {
    let mut transcript_rx = transcript_tx.subscribe();
    let mut record_rx = record_tx.subscribe();
    let transcripts_dir = session_dir
        .as_ref()
        .map(|d| d.join("transcripts"))
        .unwrap_or_else(|| PathBuf::from("/tmp/coop-transcripts"));

    let s3_meta = Arc::clone(&s3);
    let sid_meta = session_id.clone();
    let agent_type_clone = agent_type.clone();
    let labels_clone = labels.clone();

    // Upload initial session metadata.
    tokio::spawn(async move {
        let meta = SessionMeta {
            session_id: sid_meta.clone(),
            agent_type: agent_type_clone,
            started_at: now_unix_secs(),
            ended_at: None,
            exit_code: None,
            labels: labels_clone,
        };
        if let Err(e) = s3_meta.upload_meta(&sid_meta, &meta).await {
            warn!(error = %e, "s3: failed to upload session metadata");
        }
    });

    // Main subscriber task.
    let s3_main = Arc::clone(&s3);
    let sid_main = session_id.clone();
    let sd = shutdown.clone();

    tokio::spawn(async move {
        let periodic_upload = tokio::time::sleep(upload_interval);
        tokio::pin!(periodic_upload);
        let mut recording_buffer: Vec<RecordingEntry> = Vec::new();
        let recording_batch_size: usize = 50;

        loop {
            tokio::select! {
                _ = sd.cancelled() => {
                    // Final flush on shutdown.
                    flush_recording_buffer(&*s3_main, &sid_main, &mut recording_buffer).await;
                    upload_session_log(&*s3_main, &sid_main, session_log_path.as_deref()).await;
                    upload_event_logs(&*s3_main, &sid_main, session_dir.as_deref()).await;
                    break;
                }

                event = transcript_rx.recv() => {
                    match event {
                        Ok(e) => {
                            let path = transcripts_dir.join(format!("{}.jsonl", e.number));
                            let s3_path = format!("transcripts/{}.jsonl", e.number);
                            if let Err(err) = s3_main.upload_file(&sid_main, &s3_path, &path).await {
                                warn!(
                                    number = e.number,
                                    error = %err,
                                    "s3: failed to upload transcript"
                                );
                            } else {
                                info!(number = e.number, "s3: uploaded transcript");
                            }

                            // Also upload session log after each transcript save
                            // (the session log changes when context compacts).
                            upload_session_log(&*s3_main, &sid_main, session_log_path.as_deref()).await;
                        }
                        Err(broadcast::error::RecvError::Lagged(n)) => {
                            warn!("s3: transcript subscriber lagged by {n}");
                        }
                        Err(_) => break,
                    }
                }

                event = record_rx.recv() => {
                    match event {
                        Ok(entry) => {
                            recording_buffer.push(entry);
                            if recording_buffer.len() >= recording_batch_size {
                                flush_recording_buffer(&*s3_main, &sid_main, &mut recording_buffer).await;
                            }
                        }
                        Err(broadcast::error::RecvError::Lagged(n)) => {
                            warn!("s3: recording subscriber lagged by {n}");
                        }
                        Err(_) => break,
                    }
                }

                _ = &mut periodic_upload => {
                    // Periodic session log upload for crash resilience.
                    upload_session_log(&*s3_main, &sid_main, session_log_path.as_deref()).await;
                    flush_recording_buffer(&*s3_main, &sid_main, &mut recording_buffer).await;
                    upload_event_logs(&*s3_main, &sid_main, session_dir.as_deref()).await;
                    periodic_upload.as_mut().reset(tokio::time::Instant::now() + upload_interval);
                }
            }
        }
    });
}

/// Upload the session log file to S3.
async fn upload_session_log<S: S3Storage>(s3: &S, session_id: &str, log_path: Option<&Path>) {
    let Some(path) = log_path else { return };
    if !path.exists() {
        return;
    }
    if let Err(e) = s3.upload_file(session_id, "session.jsonl", path).await {
        warn!(error = %e, "s3: failed to upload session log");
    }
}

/// Upload event log files (state_events.jsonl, hook_events.jsonl) to S3.
async fn upload_event_logs<S: S3Storage>(s3: &S, session_id: &str, session_dir: Option<&Path>) {
    let Some(dir) = session_dir else { return };

    for filename in &["state_events.jsonl", "hook_events.jsonl"] {
        let path = dir.join(filename);
        if path.exists() {
            if let Err(e) = s3.upload_file(session_id, filename, &path).await {
                warn!(error = %e, file = %filename, "s3: failed to upload event log");
            }
        }
    }
}

/// Flush buffered recording entries to S3 as a JSONL append.
async fn flush_recording_buffer<S: S3Storage>(
    s3: &S,
    session_id: &str,
    buffer: &mut Vec<RecordingEntry>,
) {
    if buffer.is_empty() {
        return;
    }

    let mut data = Vec::new();
    for entry in buffer.iter() {
        if let Ok(line) = serde_json::to_string(entry) {
            data.extend_from_slice(line.as_bytes());
            data.push(b'\n');
        }
    }

    // Upload recording chunks as numbered files to avoid overwrite races.
    // The first entry's seq gives us a unique, ordered name.
    let first_seq = buffer.first().map(|e| e.seq).unwrap_or(0);
    let last_seq = buffer.last().map(|e| e.seq).unwrap_or(0);
    let chunk_name = format!("recording/{first_seq}-{last_seq}.jsonl");

    if let Err(e) = s3.upload_bytes(session_id, &chunk_name, data).await {
        warn!(error = %e, "s3: failed to upload recording chunk");
        return;
    }
    buffer.clear();
}

/// Download all transcripts from S3 into a local session directory.
///
/// Used during session resume to restore transcripts before coop starts.
/// Returns the number of transcripts downloaded.
pub async fn restore_transcripts<S: S3Storage>(
    s3: &S,
    session_id: &str,
    transcripts_dir: &Path,
) -> anyhow::Result<u32> {
    tokio::fs::create_dir_all(transcripts_dir).await?;

    let numbers = s3.list_transcripts(session_id).await?;
    let mut downloaded = 0u32;

    for number in &numbers {
        let s3_path = format!("transcripts/{number}.jsonl");
        let local_path = transcripts_dir.join(format!("{number}.jsonl"));

        // Skip if already exists locally (e.g., from a previous partial restore).
        if local_path.exists() {
            downloaded += 1;
            continue;
        }

        match s3.download_file(session_id, &s3_path, &local_path).await {
            Ok(()) => {
                downloaded += 1;
                info!(number, "s3: restored transcript from S3");
            }
            Err(e) => {
                warn!(number, error = %e, "s3: failed to download transcript");
            }
        }
    }

    Ok(downloaded)
}

/// Download the session log from S3 for resume.
///
/// Downloads to the standard Claude session log location so that
/// `--resume` can discover it. Returns the local path if successful.
pub async fn restore_session_log<S: S3Storage>(
    s3: &S,
    source_session_id: &str,
    dest_path: &Path,
) -> anyhow::Result<()> {
    if let Some(parent) = dest_path.parent() {
        tokio::fs::create_dir_all(parent).await?;
    }
    s3.download_file(source_session_id, "session.jsonl", dest_path).await?;
    info!(
        source = %source_session_id,
        dest = %dest_path.display(),
        "s3: restored session log"
    );
    Ok(())
}

#[cfg(test)]
#[path = "s3_tests.rs"]
mod tests;
