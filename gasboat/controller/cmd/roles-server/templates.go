package main

const configBeadListTmpl = `
{{define "config_bead_list"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Config Beads</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 960px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #333; }
  .btn { display: inline-block; padding: 0.4rem 0.8rem; text-decoration: none; border-radius: 4px; font-size: 0.9rem; border: none; cursor: pointer; }
  .btn-primary { background: #0d6efd; color: #fff; }
  .btn-sm { padding: 0.25rem 0.5rem; font-size: 0.8rem; }
  .btn-outline { border: 1px solid #6c757d; color: #6c757d; background: transparent; }
  .btn-danger { border: 1px solid #dc3545; color: #dc3545; background: transparent; }
  table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  th, td { text-align: left; padding: 0.6rem 0.8rem; border-bottom: 1px solid #e9ecef; }
  th { background: #f1f3f5; font-weight: 600; color: #495057; }
  .label { display: inline-block; background: #e9ecef; padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.8rem; margin-right: 0.2rem; }
  .actions { white-space: nowrap; }
  .actions a { margin-right: 0.3rem; }
  .toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; }
</style>
</head>
<body>
{{template "nav"}}
<div class="toolbar">
  <h1>Config Beads</h1>
  <a href="/config-beads/new" class="btn btn-primary">New Config Bead</a>
</div>
<table>
  <thead>
    <tr><th>ID</th><th>Category</th><th>Labels</th><th>Actions</th></tr>
  </thead>
  <tbody>
  {{range .}}
    <tr>
      <td><code>{{.ID}}</code></td>
      <td>{{.Title}}</td>
      <td>{{range .Labels}}<span class="label">{{.}}</span>{{end}}</td>
      <td class="actions">
        <a href="/config-beads/{{.ID}}/edit" class="btn btn-sm btn-outline">Edit</a>
        {{if eq .Title "claude-instructions"}}<a href="/config-beads/{{.ID}}/instructions" class="btn btn-sm btn-outline">Sections</a>{{end}}
        <a href="/config-beads/{{.ID}}/delete" class="btn btn-sm btn-danger">Delete</a>
      </td>
    </tr>
  {{else}}
    <tr><td colspan="4">No config beads found.</td></tr>
  {{end}}
  </tbody>
</table>
</body>
</html>
{{end}}
`

const configBeadFormTmpl = `
{{define "config_bead_form"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{if .IsEdit}}Edit{{else}}New{{end}} Config Bead</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 700px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #333; }
  .form-group { margin-bottom: 1rem; }
  label { display: block; font-weight: 600; margin-bottom: 0.3rem; color: #495057; }
  select, input[type=text], textarea { width: 100%; padding: 0.5rem; border: 1px solid #ced4da; border-radius: 4px; font-size: 0.95rem; box-sizing: border-box; }
  textarea { font-family: monospace; min-height: 200px; resize: vertical; }
  .hint { font-size: 0.8rem; color: #6c757d; margin-top: 0.2rem; }
  .error { background: #f8d7da; border: 1px solid #f5c2c7; color: #842029; padding: 0.6rem 1rem; border-radius: 4px; margin-bottom: 1rem; }
  .btn { display: inline-block; padding: 0.5rem 1rem; text-decoration: none; border-radius: 4px; font-size: 0.95rem; border: none; cursor: pointer; }
  .btn-primary { background: #0d6efd; color: #fff; }
  .btn-secondary { background: #6c757d; color: #fff; }
  .toolbar { display: flex; gap: 0.5rem; }
</style>
</head>
<body>
{{template "nav"}}
<h1>{{if .IsEdit}}Edit{{else}}New{{end}} Config Bead</h1>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
<form method="POST">
  <div class="form-group">
    <label for="title">Category</label>
    <select name="title" id="title">
      <option value="">-- select category --</option>
      {{range .Categories}}
      <option value="{{.}}"{{if eq . $.Title}} selected{{end}}>{{.}}</option>
      {{end}}
    </select>
    <div class="hint">Config bead category (e.g., claude-settings, claude-instructions).</div>
  </div>
  <div class="form-group">
    <label for="labels">Labels</label>
    <input type="text" name="labels" id="labels" value="{{.Labels}}" placeholder="global, role:crew, project:gasboat">
    <div class="hint">Comma-separated labels for scope targeting. More specific labels override less specific ones: global &lt; rig:X &lt; role:X &lt; agent:X.</div>
  </div>
  <div class="form-group">
    <label for="value">Value (JSON)</label>
    <textarea name="value" id="value" placeholder='{"model": "sonnet", "permissions": {...}}'>{{.Value}}</textarea>
    <div class="hint">JSON object with the configuration value for this bead.</div>
  </div>
  <div class="toolbar">
    <button type="submit" class="btn btn-primary">{{if .IsEdit}}Save Changes{{else}}Create{{end}}</button>
    <a href="/config-beads" class="btn btn-secondary">Cancel</a>
  </div>
</form>
</body>
</html>
{{end}}
`

const configBeadDeleteConfirmTmpl = `
{{define "config_bead_delete_confirm"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Delete Config Bead</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 600px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #842029; }
  .card { background: #fff; border: 1px solid #e9ecef; border-radius: 8px; padding: 1.2rem; margin-bottom: 1rem; }
  .field { margin-bottom: 0.5rem; }
  .field-label { font-weight: 600; color: #495057; }
  .label { display: inline-block; background: #e9ecef; padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.8rem; margin-right: 0.2rem; }
  .btn { display: inline-block; padding: 0.5rem 1rem; text-decoration: none; border-radius: 4px; font-size: 0.95rem; border: none; cursor: pointer; }
  .btn-danger { background: #dc3545; color: #fff; }
  .btn-secondary { background: #6c757d; color: #fff; }
  .toolbar { display: flex; gap: 0.5rem; }
</style>
</head>
<body>
{{template "nav"}}
<h1>Delete Config Bead</h1>
<p>Are you sure you want to delete this config bead? This action cannot be undone.</p>
<div class="card">
  <div class="field"><span class="field-label">ID:</span> <code>{{.ID}}</code></div>
  <div class="field"><span class="field-label">Category:</span> {{.Title}}</div>
  <div class="field"><span class="field-label">Labels:</span> {{range .Labels}}<span class="label">{{.}}</span>{{end}}</div>
</div>
<form method="POST">
  <div class="toolbar">
    <button type="submit" class="btn btn-danger">Delete</button>
    <a href="/config-beads" class="btn btn-secondary">Cancel</a>
  </div>
</form>
</body>
</html>
{{end}}
`

const adviceListTmpl = `
{{define "advice_list"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Advice Beads</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 960px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #333; }
  .btn { display: inline-block; padding: 0.4rem 0.8rem; text-decoration: none; border-radius: 4px; font-size: 0.9rem; border: none; cursor: pointer; }
  .btn-primary { background: #0d6efd; color: #fff; }
  .btn-sm { padding: 0.25rem 0.5rem; font-size: 0.8rem; }
  .btn-outline { border: 1px solid #6c757d; color: #6c757d; background: transparent; }
  .btn-danger { border: 1px solid #dc3545; color: #dc3545; background: transparent; }
  table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  th, td { text-align: left; padding: 0.6rem 0.8rem; border-bottom: 1px solid #e9ecef; }
  th { background: #f1f3f5; font-weight: 600; color: #495057; }
  .label { display: inline-block; background: #e9ecef; padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.8rem; margin-right: 0.2rem; }
  .status { display: inline-block; padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.8rem; background: #d1e7dd; color: #0f5132; }
  .actions { white-space: nowrap; }
  .actions a { margin-right: 0.3rem; }
  .toolbar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; }
</style>
</head>
<body>
{{template "nav"}}
<div class="toolbar">
  <h1>Advice Beads</h1>
  <a href="/advice/new" class="btn btn-primary">New Advice</a>
</div>
<table>
  <thead>
    <tr><th>ID</th><th>Title</th><th>Labels</th><th>Status</th><th>Actions</th></tr>
  </thead>
  <tbody>
  {{range .}}
    <tr>
      <td><code>{{.ID}}</code></td>
      <td>{{.Title}}</td>
      <td>{{range .Labels}}<span class="label">{{.}}</span>{{end}}</td>
      <td><span class="status">{{.Status}}</span></td>
      <td class="actions">
        <a href="/advice/{{.ID}}/edit" class="btn btn-sm btn-outline">Edit</a>
        <a href="/advice/{{.ID}}/delete" class="btn btn-sm btn-danger">Delete</a>
      </td>
    </tr>
  {{else}}
    <tr><td colspan="5">No advice beads found.</td></tr>
  {{end}}
  </tbody>
</table>
</body>
</html>
{{end}}
`

const adviceFormTmpl = `
{{define "advice_form"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{if .IsEdit}}Edit{{else}}New{{end}} Advice</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 700px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #333; }
  .form-group { margin-bottom: 1rem; }
  label { display: block; font-weight: 600; margin-bottom: 0.3rem; color: #495057; }
  input[type=text], textarea { width: 100%; padding: 0.5rem; border: 1px solid #ced4da; border-radius: 4px; font-size: 0.95rem; box-sizing: border-box; }
  textarea { min-height: 200px; resize: vertical; }
  .hint { font-size: 0.8rem; color: #6c757d; margin-top: 0.2rem; }
  .error { background: #f8d7da; border: 1px solid #f5c2c7; color: #842029; padding: 0.6rem 1rem; border-radius: 4px; margin-bottom: 1rem; }
  .btn { display: inline-block; padding: 0.5rem 1rem; text-decoration: none; border-radius: 4px; font-size: 0.95rem; border: none; cursor: pointer; }
  .btn-primary { background: #0d6efd; color: #fff; }
  .btn-secondary { background: #6c757d; color: #fff; }
  .toolbar { display: flex; gap: 0.5rem; }
</style>
</head>
<body>
{{template "nav"}}
<h1>{{if .IsEdit}}Edit{{else}}New{{end}} Advice</h1>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
<form method="POST">
  <div class="form-group">
    <label for="title">Title</label>
    <input type="text" name="title" id="title" value="{{.Title}}" placeholder="Short descriptive title">
    <div class="hint">A brief name for this advice (e.g., "Use gb prime for context recovery").</div>
  </div>
  <div class="form-group">
    <label for="labels">Labels</label>
    <input type="text" name="labels" id="labels" value="{{.Labels}}" placeholder="global, role:crew, project:gasboat">
    <div class="hint">Comma-separated labels. Advice is shown to agents matching these labels.</div>
  </div>
  <div class="form-group">
    <label for="description">Description</label>
    <textarea name="description" id="description" placeholder="Detailed advice content shown to agents...">{{.Description}}</textarea>
    <div class="hint">The full advice text injected into agent context. Supports markdown.</div>
  </div>
  <div class="toolbar">
    <button type="submit" class="btn btn-primary">{{if .IsEdit}}Save Changes{{else}}Create{{end}}</button>
    <a href="/advice" class="btn btn-secondary">Cancel</a>
  </div>
</form>
</body>
</html>
{{end}}
`

const adviceDeleteConfirmTmpl = `
{{define "advice_delete_confirm"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Delete Advice</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 600px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #842029; }
  .card { background: #fff; border: 1px solid #e9ecef; border-radius: 8px; padding: 1.2rem; margin-bottom: 1rem; }
  .field { margin-bottom: 0.5rem; }
  .field-label { font-weight: 600; color: #495057; }
  .label { display: inline-block; background: #e9ecef; padding: 0.15rem 0.4rem; border-radius: 3px; font-size: 0.8rem; margin-right: 0.2rem; }
  .btn { display: inline-block; padding: 0.5rem 1rem; text-decoration: none; border-radius: 4px; font-size: 0.95rem; border: none; cursor: pointer; }
  .btn-danger { background: #dc3545; color: #fff; }
  .btn-secondary { background: #6c757d; color: #fff; }
  .toolbar { display: flex; gap: 0.5rem; }
</style>
</head>
<body>
{{template "nav"}}
<h1>Delete Advice</h1>
<p>Are you sure you want to delete this advice bead? This action cannot be undone.</p>
<div class="card">
  <div class="field"><span class="field-label">ID:</span> <code>{{.ID}}</code></div>
  <div class="field"><span class="field-label">Title:</span> {{.Title}}</div>
  <div class="field"><span class="field-label">Labels:</span> {{range .Labels}}<span class="label">{{.}}</span>{{end}}</div>
</div>
<form method="POST">
  <div class="toolbar">
    <button type="submit" class="btn btn-danger">Delete</button>
    <a href="/advice" class="btn btn-secondary">Cancel</a>
  </div>
</form>
</body>
</html>
{{end}}
`

const instructionsFormTmpl = `
{{define "instructions_form"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Edit Claude Instructions</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 900px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #333; }
  h2 { color: #495057; font-size: 1.1rem; margin-top: 1.5rem; margin-bottom: 0.3rem; border-bottom: 1px solid #dee2e6; padding-bottom: 0.3rem; }
  .meta { color: #6c757d; font-size: 0.9rem; margin-bottom: 1rem; }
  .form-group { margin-bottom: 1rem; }
  textarea { width: 100%; padding: 0.5rem; border: 1px solid #ced4da; border-radius: 4px; font-size: 0.9rem; box-sizing: border-box; min-height: 120px; resize: vertical; font-family: monospace; }
  .hint { font-size: 0.8rem; color: #6c757d; margin-top: 0.2rem; }
  .error { background: #f8d7da; border: 1px solid #f5c2c7; color: #842029; padding: 0.6rem 1rem; border-radius: 4px; margin-bottom: 1rem; }
  .btn { display: inline-block; padding: 0.5rem 1rem; text-decoration: none; border-radius: 4px; font-size: 0.95rem; border: none; cursor: pointer; }
  .btn-primary { background: #0d6efd; color: #fff; }
  .btn-secondary { background: #6c757d; color: #fff; }
  .toolbar { display: flex; gap: 0.5rem; margin-top: 1.5rem; }
</style>
</head>
<body>
{{template "nav"}}
<h1>Edit Claude Instructions</h1>
<div class="meta">Bead ID: <code>{{.ID}}</code> | Labels: {{.Labels}}</div>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
<form method="POST">
  {{range .Sections}}
  <h2>{{.Label}}</h2>
  <div class="form-group">
    <textarea name="section_{{.Key}}" placeholder="{{.Hint}}">{{.Value}}</textarea>
    <div class="hint">{{.Hint}}</div>
  </div>
  {{end}}
  <div class="toolbar">
    <button type="submit" class="btn btn-primary">Save Changes</button>
    <a href="/config-beads" class="btn btn-secondary">Cancel</a>
  </div>
</form>
</body>
</html>
{{end}}
`
