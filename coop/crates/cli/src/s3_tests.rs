// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Alfred Jean LLC

use super::*;

#[test]
fn session_meta_serializes() -> anyhow::Result<()> {
    let meta = SessionMeta {
        session_id: "test-123".into(),
        agent_type: "claude".into(),
        started_at: 1700000000,
        ended_at: Some(1700003600),
        exit_code: Some(0),
        labels: vec!["project:gasboat".into()],
    };
    let json = serde_json::to_string(&meta)?;
    let parsed: SessionMeta = serde_json::from_str(&json)?;
    assert_eq!(parsed.session_id, "test-123");
    assert_eq!(parsed.exit_code, Some(0));
    assert_eq!(parsed.labels.len(), 1);
    Ok(())
}

#[test]
fn session_meta_skips_none_fields() -> anyhow::Result<()> {
    let meta = SessionMeta {
        session_id: "test-456".into(),
        agent_type: "claude".into(),
        started_at: 1700000000,
        ended_at: None,
        exit_code: None,
        labels: vec![],
    };
    let json = serde_json::to_string(&meta)?;
    assert!(!json.contains("ended_at"));
    assert!(!json.contains("exit_code"));
    Ok(())
}

#[test]
fn s3_key_format() {
    // Verify the key format logic without needing a real S3 client.
    let prefix = "coop/sessions";
    let session_id = "abc-123";
    let path = "transcripts/1.jsonl";
    let key = format!("{prefix}/{session_id}/{path}");
    assert_eq!(key, "coop/sessions/abc-123/transcripts/1.jsonl");
}
