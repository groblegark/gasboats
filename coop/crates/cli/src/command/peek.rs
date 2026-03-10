// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Alfred Jean LLC

//! `coop peek` — view mux session screens from the CLI.
//!
//! Talks to the mux server's session endpoints via `COOP_MUX_URL`.
//! With `--archived`, queries S3 for historical session data instead.

/// CLI arguments for `coop peek`.
#[derive(Debug, clap::Args)]
pub struct PeekArgs {
    /// Session ID to peek at. Omit to list all sessions.
    pub session: Option<String>,

    /// Show ANSI-colored output (default: true if terminal).
    #[arg(long)]
    pub ansi: Option<bool>,

    /// Show plain text (no ANSI escape codes).
    #[arg(long, conflicts_with = "ansi")]
    pub plain: bool,

    /// Query archived sessions from S3 instead of live mux.
    /// Reads COOP_S3_BUCKET, COOP_S3_PREFIX, and COOP_S3_REGION env vars.
    #[arg(long, short = 'a')]
    pub archived: bool,

    /// Show transcripts for an archived session.
    #[arg(long, requires = "session")]
    pub transcripts: bool,

    /// Show recording chunks for an archived session.
    #[arg(long, requires = "session")]
    pub recording: bool,
}

/// Run the `coop peek` subcommand. Returns a process exit code.
pub async fn run(args: &PeekArgs) -> i32 {
    #[cfg(feature = "s3")]
    if args.archived {
        return run_archived(args).await;
    }

    #[cfg(not(feature = "s3"))]
    if args.archived {
        eprintln!("error: --archived requires coop built with the 's3' feature");
        return 2;
    }

    let mux_url = match std::env::var("COOP_MUX_URL") {
        Ok(u) => u.trim_end_matches('/').to_owned(),
        Err(_) => {
            eprintln!("error: COOP_MUX_URL is not set");
            return 2;
        }
    };

    let mux_token = std::env::var("COOP_MUX_TOKEN").ok();
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(10))
        .build()
        .unwrap_or_default();

    match &args.session {
        Some(id) => cmd_screen(&client, &mux_url, mux_token.as_deref(), id, args).await,
        None => cmd_list(&client, &mux_url, mux_token.as_deref()).await,
    }
}

fn apply_auth(req: reqwest::RequestBuilder, token: Option<&str>) -> reqwest::RequestBuilder {
    match token {
        Some(t) => req.bearer_auth(t),
        None => req,
    }
}

async fn cmd_list(client: &reqwest::Client, mux_url: &str, token: Option<&str>) -> i32 {
    let url = format!("{mux_url}/api/v1/sessions");
    let resp = match apply_auth(client.get(&url), token).send().await {
        Ok(r) => r,
        Err(e) => {
            eprintln!("error: {e}");
            return 1;
        }
    };

    let status = resp.status();
    let text = resp.text().await.unwrap_or_default();

    if status.is_success() {
        match serde_json::from_str::<Vec<serde_json::Value>>(&text) {
            Ok(sessions) => {
                if sessions.is_empty() {
                    println!("No sessions registered.");
                } else {
                    println!("{:<38} {:<30} {:<12} HEALTH", "SESSION ID", "POD", "STATE");
                    println!("{}", "-".repeat(88));
                    for s in &sessions {
                        let id = s.get("id").and_then(|v| v.as_str()).unwrap_or("?");
                        let pod = s
                            .get("metadata")
                            .and_then(|m| m.get("k8s"))
                            .and_then(|k| k.get("pod"))
                            .and_then(|v| v.as_str())
                            .unwrap_or("-");
                        let state =
                            s.get("cached_state").and_then(|v| v.as_str()).unwrap_or("unknown");
                        let failures =
                            s.get("health_failures").and_then(|v| v.as_u64()).unwrap_or(0);
                        let health = if failures == 0 {
                            "ok".to_string()
                        } else {
                            format!("{failures} failures")
                        };
                        println!("{id:<38} {pod:<30} {state:<12} {health}");
                    }
                    println!("\n{} session(s)", sessions.len());
                }
                0
            }
            Err(_) => {
                println!("{text}");
                0
            }
        }
    } else {
        eprintln!("error ({status}): {text}");
        1
    }
}

async fn cmd_screen(
    client: &reqwest::Client,
    mux_url: &str,
    token: Option<&str>,
    session_id: &str,
    args: &PeekArgs,
) -> i32 {
    // Allow partial session ID matching: if input is short, list sessions and find a match.
    let resolved_id = if session_id.len() < 36 {
        match resolve_session_id(client, mux_url, token, session_id).await {
            Ok(id) => id,
            Err(code) => return code,
        }
    } else {
        session_id.to_owned()
    };

    let url = format!("{mux_url}/api/v1/sessions/{resolved_id}/screen");
    let resp = match apply_auth(client.get(&url), token).send().await {
        Ok(r) => r,
        Err(e) => {
            eprintln!("error: {e}");
            return 1;
        }
    };

    let status = resp.status();
    let text = resp.text().await.unwrap_or_default();

    if !status.is_success() {
        eprintln!("error ({status}): {text}");
        return 1;
    }

    match serde_json::from_str::<serde_json::Value>(&text) {
        Ok(screen) => {
            let use_ansi = match (args.plain, args.ansi) {
                (true, _) => false,
                (_, Some(v)) => v,
                _ => std::io::IsTerminal::is_terminal(&std::io::stdout()),
            };

            let lines_key = if use_ansi { "ansi" } else { "lines" };
            if let Some(lines) = screen.get(lines_key).and_then(|v| v.as_array()) {
                for line in lines {
                    if let Some(s) = line.as_str() {
                        println!("{s}");
                    }
                }
            } else if let Some(lines) = screen.get("lines").and_then(|v| v.as_array()) {
                // Fallback to plain lines if ansi not available.
                for line in lines {
                    if let Some(s) = line.as_str() {
                        println!("{s}");
                    }
                }
            } else {
                println!("{text}");
            }

            // Show dimensions in stderr so they don't pollute piped output.
            let cols = screen.get("cols").and_then(|v| v.as_u64()).unwrap_or(0);
            let rows = screen.get("rows").and_then(|v| v.as_u64()).unwrap_or(0);
            if cols > 0 && rows > 0 {
                eprintln!("[{cols}x{rows}]");
            }
            0
        }
        Err(_) => {
            println!("{text}");
            0
        }
    }
}

/// Resolve a partial session ID or pod name to a full session ID.
async fn resolve_session_id(
    client: &reqwest::Client,
    mux_url: &str,
    token: Option<&str>,
    partial: &str,
) -> Result<String, i32> {
    let url = format!("{mux_url}/api/v1/sessions");
    let resp = match apply_auth(client.get(&url), token).send().await {
        Ok(r) => r,
        Err(e) => {
            eprintln!("error listing sessions: {e}");
            return Err(1);
        }
    };

    let text = resp.text().await.unwrap_or_default();
    let sessions: Vec<serde_json::Value> = match serde_json::from_str(&text) {
        Ok(v) => v,
        Err(e) => {
            eprintln!("error parsing sessions: {e}");
            return Err(1);
        }
    };

    let partial_lower = partial.to_lowercase();
    let mut matches: Vec<String> = Vec::new();

    for s in &sessions {
        let id = s.get("id").and_then(|v| v.as_str()).unwrap_or("");
        let pod = s
            .get("metadata")
            .and_then(|m| m.get("k8s"))
            .and_then(|k| k.get("pod"))
            .and_then(|v| v.as_str())
            .unwrap_or("");

        if id.starts_with(&partial_lower) || pod.to_lowercase().contains(&partial_lower) {
            matches.push(id.to_owned());
        }
    }

    match matches.len() {
        0 => {
            eprintln!("error: no session matching '{partial}'");
            Err(1)
        }
        1 => matches.into_iter().next().ok_or(1),
        n => {
            eprintln!("error: '{partial}' matches {n} sessions:");
            for id in &matches {
                // Find the pod name for this id.
                let pod = sessions
                    .iter()
                    .find(|s| s.get("id").and_then(|v| v.as_str()) == Some(id))
                    .and_then(|s| s.get("metadata"))
                    .and_then(|m| m.get("k8s"))
                    .and_then(|k| k.get("pod"))
                    .and_then(|v| v.as_str())
                    .unwrap_or("-");
                eprintln!("  {id}  ({pod})");
            }
            Err(1)
        }
    }
}

// ---------------------------------------------------------------------------
// Archived session support (S3)
// ---------------------------------------------------------------------------

#[cfg(feature = "s3")]
async fn run_archived(args: &PeekArgs) -> i32 {
    use crate::s3::{S3Client, S3Storage};

    let bucket = match std::env::var("COOP_S3_BUCKET") {
        Ok(b) if !b.is_empty() => b,
        _ => {
            eprintln!("error: COOP_S3_BUCKET is not set (required for --archived)");
            return 2;
        }
    };
    let prefix = std::env::var("COOP_S3_PREFIX").unwrap_or_else(|_| "coop/sessions".into());
    let region = std::env::var("COOP_S3_REGION").ok();

    let s3 = match S3Client::new(bucket, prefix, region).await {
        Ok(c) => c,
        Err(e) => {
            eprintln!("error: failed to create S3 client: {e}");
            return 1;
        }
    };

    match &args.session {
        None => archived_list(&s3).await,
        Some(session_id) => {
            if args.transcripts {
                archived_transcripts(&s3, session_id).await
            } else if args.recording {
                archived_recording(&s3, session_id).await
            } else {
                archived_meta(&s3, session_id).await
            }
        }
    }
}

/// List all archived sessions from S3.
#[cfg(feature = "s3")]
async fn archived_list<S: crate::s3::S3Storage>(s3: &S) -> i32 {
    let session_ids = match s3.list_sessions().await {
        Ok(ids) => ids,
        Err(e) => {
            eprintln!("error listing archived sessions: {e}");
            return 1;
        }
    };

    if session_ids.is_empty() {
        println!("No archived sessions found.");
        return 0;
    }

    println!("{:<38} {:<12} {:<20} {:<6} LABELS", "SESSION ID", "AGENT", "STARTED", "EXIT");
    println!("{}", "-".repeat(90));

    for sid in &session_ids {
        match s3.download_meta(sid).await {
            Ok(Some(meta)) => {
                let started = format_unix_ts(meta.started_at);
                let exit = meta.exit_code.map(|c| c.to_string()).unwrap_or_else(|| "-".into());
                let labels =
                    if meta.labels.is_empty() { "-".into() } else { meta.labels.join(", ") };
                println!(
                    "{:<38} {:<12} {:<20} {:<6} {labels}",
                    meta.session_id, meta.agent_type, started, exit
                );
            }
            Ok(None) => {
                println!("{sid:<38} {:<12} {:<20} {:<6} -", "?", "?", "-");
            }
            Err(_) => {
                println!("{sid:<38} {:<12} {:<20} {:<6} -", "?", "?", "-");
            }
        }
    }

    println!("\n{} archived session(s)", session_ids.len());
    0
}

/// Show metadata for an archived session.
#[cfg(feature = "s3")]
async fn archived_meta<S: crate::s3::S3Storage>(s3: &S, session_id: &str) -> i32 {
    let meta = match s3.download_meta(session_id).await {
        Ok(Some(m)) => m,
        Ok(None) => {
            eprintln!("error: no archived session '{session_id}'");
            return 1;
        }
        Err(e) => {
            eprintln!("error fetching session metadata: {e}");
            return 1;
        }
    };

    println!("Session:    {}", meta.session_id);
    println!("Agent:      {}", meta.agent_type);
    println!("Started:    {}", format_unix_ts(meta.started_at));
    if let Some(ended) = meta.ended_at {
        println!("Ended:      {}", format_unix_ts(ended));
    }
    if let Some(code) = meta.exit_code {
        println!("Exit code:  {code}");
    }
    if !meta.labels.is_empty() {
        println!("Labels:     {}", meta.labels.join(", "));
    }

    // Show available artifacts.
    let transcripts = s3.list_transcripts(session_id).await.unwrap_or_default();
    let recordings = s3.list_recording_chunks(session_id).await.unwrap_or_default();
    let has_session_log = s3.exists(session_id, "session.jsonl").await;

    println!();
    println!("Artifacts:");
    println!(
        "  transcripts:  {} file(s){}",
        transcripts.len(),
        if transcripts.is_empty() { "" } else { " (use --transcripts to view)" }
    );
    println!(
        "  recording:    {} chunk(s){}",
        recordings.len(),
        if recordings.is_empty() { "" } else { " (use --recording to view)" }
    );
    println!("  session log:  {}", if has_session_log { "yes" } else { "no" });

    0
}

/// List and optionally show archived transcripts.
#[cfg(feature = "s3")]
async fn archived_transcripts<S: crate::s3::S3Storage>(s3: &S, session_id: &str) -> i32 {
    let numbers = match s3.list_transcripts(session_id).await {
        Ok(n) => n,
        Err(e) => {
            eprintln!("error listing transcripts: {e}");
            return 1;
        }
    };

    if numbers.is_empty() {
        println!("No transcripts found for session '{session_id}'.");
        return 0;
    }

    println!("{} transcript(s) for session {session_id}:", numbers.len());
    println!();

    for number in &numbers {
        let s3_path = format!("transcripts/{number}.jsonl");
        match s3.download_bytes(session_id, &s3_path).await {
            Ok(data) => {
                let content = String::from_utf8_lossy(&data);
                let line_count = content.lines().count();
                println!("--- transcript {number} ({line_count} lines) ---");
                print!("{content}");
                if !content.ends_with('\n') {
                    println!();
                }
            }
            Err(e) => {
                eprintln!("  #{number}: error: {e}");
            }
        }
    }

    0
}

/// List and optionally show archived recording chunks.
#[cfg(feature = "s3")]
async fn archived_recording<S: crate::s3::S3Storage>(s3: &S, session_id: &str) -> i32 {
    let chunks = match s3.list_recording_chunks(session_id).await {
        Ok(c) => c,
        Err(e) => {
            eprintln!("error listing recording chunks: {e}");
            return 1;
        }
    };

    if chunks.is_empty() {
        println!("No recording chunks found for session '{session_id}'.");
        return 0;
    }

    println!("{} recording chunk(s) for session {session_id}:", chunks.len());
    println!();

    for chunk in &chunks {
        let s3_path = format!("recording/{chunk}");
        match s3.download_bytes(session_id, &s3_path).await {
            Ok(data) => {
                let content = String::from_utf8_lossy(&data);
                let entry_count = content.lines().count();
                println!("--- {chunk} ({entry_count} entries) ---");
                print!("{content}");
                if !content.ends_with('\n') {
                    println!();
                }
            }
            Err(e) => {
                eprintln!("  {chunk}: error: {e}");
            }
        }
    }

    0
}

/// Format a Unix timestamp as a human-readable date-time.
#[cfg(feature = "s3")]
fn format_unix_ts(ts: u64) -> String {
    // Simple formatting without chrono dependency.
    // Produces "2026-01-15 12:30:00" style output.
    let secs = ts;
    let days = secs / 86400;
    let time_secs = secs % 86400;
    let hours = time_secs / 3600;
    let minutes = (time_secs % 3600) / 60;
    let seconds = time_secs % 60;

    // Days since Unix epoch → year/month/day (simplified civil calendar).
    let (year, month, day) = days_to_civil(days);
    format!("{year:04}-{month:02}-{day:02} {hours:02}:{minutes:02}:{seconds:02}")
}

/// Convert days since Unix epoch to (year, month, day).
/// Algorithm from Howard Hinnant's chrono-compatible date library.
#[cfg(feature = "s3")]
fn days_to_civil(days: u64) -> (i64, u32, u32) {
    let z = days as i64 + 719468;
    let era = if z >= 0 { z } else { z - 146096 } / 146097;
    let doe = (z - era * 146097) as u32;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let y = yoe as i64 + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = if m <= 2 { y + 1 } else { y };
    (y, m, d)
}
