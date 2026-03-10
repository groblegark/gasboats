package main

const rolesListTmpl = `
{{define "roles_list"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Roles</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 960px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #333; }
  table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  th, td { text-align: left; padding: 0.6rem 0.8rem; border-bottom: 1px solid #e9ecef; }
  th { background: #f1f3f5; font-weight: 600; color: #495057; }
  .badge { display: inline-block; background: #e9ecef; padding: 0.15rem 0.5rem; border-radius: 3px; font-size: 0.85rem; }
  .badge-primary { background: #cfe2ff; color: #084298; }
  a { color: #0d6efd; text-decoration: none; }
  a:hover { text-decoration: underline; }
</style>
</head>
<body>
{{template "nav"}}
<h1>Roles</h1>
<table>
  <thead>
    <tr><th>Role</th><th>Config Beads</th><th>Advice Beads</th><th>Active Agents</th></tr>
  </thead>
  <tbody>
  {{range .Roles}}
    <tr>
      <td><a href="/roles/{{.Name}}">{{.Name}}</a></td>
      <td><span class="badge">{{.ConfigCount}}</span></td>
      <td><span class="badge">{{.AdviceCount}}</span></td>
      <td><span class="badge badge-primary">{{.AgentCount}}</span></td>
    </tr>
  {{else}}
    <tr><td colspan="4">No roles found.</td></tr>
  {{end}}
  </tbody>
</table>
</body>
</html>
{{end}}
`

const rolePreviewTmpl = `
{{define "role_preview"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Role: {{.Name}}</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 960px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #333; }
  h2 { color: #495057; font-size: 1.2rem; margin-top: 1.5rem; border-bottom: 1px solid #dee2e6; padding-bottom: 0.3rem; }
  .meta { color: #6c757d; font-size: 0.9rem; margin-bottom: 1rem; }
  .card { background: #fff; border: 1px solid #e9ecef; border-radius: 8px; padding: 1rem; margin-bottom: 0.8rem; }
  .card-title { font-weight: 600; color: #212529; margin-bottom: 0.3rem; }
  .card-meta { font-size: 0.8rem; color: #6c757d; margin-bottom: 0.5rem; }
  .label { display: inline-block; background: #e9ecef; padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.8rem; margin-right: 0.2rem; }
  pre { background: #f1f3f5; padding: 0.8rem; border-radius: 4px; font-size: 0.85rem; overflow-x: auto; max-height: 300px; white-space: pre-wrap; word-wrap: break-word; }
  .agent-list { display: flex; flex-wrap: wrap; gap: 0.4rem; }
  .agent-badge { background: #d1e7dd; color: #0f5132; padding: 0.2rem 0.5rem; border-radius: 3px; font-size: 0.85rem; }
  .empty { color: #6c757d; font-style: italic; }
  a { color: #0d6efd; text-decoration: none; }
  a:hover { text-decoration: underline; }
  .back { margin-bottom: 1rem; display: inline-block; }
</style>
</head>
<body>
{{template "nav"}}
<a href="/roles" class="back">&larr; All Roles</a>
<h1>Role: {{.Name}}</h1>

<h2>Active Agents ({{len .ActiveAgents}})</h2>
{{if .ActiveAgents}}
<div class="agent-list">
  {{range .ActiveAgents}}<span class="agent-badge">{{.}}</span>{{end}}
</div>
{{else}}
<p class="empty">No active agents with this role.</p>
{{end}}

<h2>Config Beads ({{len .ConfigBeads}})</h2>
{{range .ConfigBeads}}
<div class="card">
  <div class="card-title">{{.Title}}</div>
  <div class="card-meta">
    <code>{{.ID}}</code>
    {{range .Labels}}<span class="label">{{.}}</span>{{end}}
  </div>
  {{if .Value}}<pre>{{.Value}}</pre>{{end}}
</div>
{{else}}
<p class="empty">No config beads for this role.</p>
{{end}}

<h2>Advice Beads ({{len .AdviceBeads}})</h2>
{{range .AdviceBeads}}
<div class="card">
  <div class="card-title">{{.Title}}</div>
  <div class="card-meta">
    <code>{{.ID}}</code>
    {{range .Labels}}<span class="label">{{.}}</span>{{end}}
  </div>
  {{if .Description}}<pre>{{.Description}}</pre>{{end}}
</div>
{{else}}
<p class="empty">No advice beads for this role.</p>
{{end}}
</body>
</html>
{{end}}
`
