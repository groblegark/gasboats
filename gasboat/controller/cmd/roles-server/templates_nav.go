package main

const navTmpl = `
{{define "nav"}}
<nav style="background:#212529;padding:0.5rem 1rem;margin:-1rem -1rem 1rem -1rem;display:flex;gap:1.5rem;align-items:center;">
  <a href="/" style="color:#fff;text-decoration:none;font-weight:700;font-size:1.1rem;">Roles Server</a>
  <a href="/config-beads" style="color:#adb5bd;text-decoration:none;">Config Beads</a>
  <a href="/advice" style="color:#adb5bd;text-decoration:none;">Advice</a>
  <a href="/roles" style="color:#adb5bd;text-decoration:none;">Roles</a>
  <a href="/api/roles" style="color:#adb5bd;text-decoration:none;">API</a>
</nav>
{{end}}
`

const indexTmpl = `
{{define "index"}}
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Roles Server</title>
<style>
  body { font-family: system-ui, sans-serif; max-width: 960px; margin: 0 auto; padding: 1rem; background: #f8f9fa; }
  h1 { color: #333; }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 1rem; margin-top: 1rem; }
  .card { background: #fff; border: 1px solid #e9ecef; border-radius: 8px; padding: 1.2rem; text-decoration: none; color: inherit; transition: box-shadow 0.15s; }
  .card:hover { box-shadow: 0 2px 8px rgba(0,0,0,0.12); }
  .card-title { font-size: 0.9rem; color: #6c757d; margin-bottom: 0.3rem; }
  .card-value { font-size: 2rem; font-weight: 700; color: #212529; }
  .card-link { display: block; margin-top: 0.5rem; color: #0d6efd; font-size: 0.85rem; }
</style>
</head>
<body>
{{template "nav"}}
<h1>Dashboard</h1>
<div class="cards">
  <a href="/config-beads" class="card">
    <div class="card-title">Config Beads</div>
    <div class="card-value">{{.ConfigCount}}</div>
    <span class="card-link">Manage config beads</span>
  </a>
  <a href="/advice" class="card">
    <div class="card-title">Advice Beads</div>
    <div class="card-value">{{.AdviceCount}}</div>
    <span class="card-link">Manage advice beads</span>
  </a>
  <a href="/roles" class="card">
    <div class="card-title">Roles</div>
    <div class="card-value">{{.RoleCount}}</div>
    <span class="card-link">View roles</span>
  </a>
  <div class="card">
    <div class="card-title">Active Agents</div>
    <div class="card-value">{{.AgentCount}}</div>
  </div>
</div>
</body>
</html>
{{end}}
`
