document.addEventListener('DOMContentLoaded', () => {
  // ── Page Navigation ──
  document.querySelectorAll('.nav-links a').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const page = link.dataset.page;
      showPage(page);
    });
  });

  // Initial data fetch
  fetchHealth();
  fetchAnalyticsStats();

  // Poll every 5 seconds
  setInterval(() => {
    fetchHealth();
    fetchAnalyticsStats();
  }, 5000);
});

function showPage(pageName) {
  // Hide all pages
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active-page'));
  // Show target
  const target = document.getElementById('page-' + pageName);
  if (target) target.classList.add('active-page');
  // Update nav
  document.querySelectorAll('.nav-links a').forEach(a => {
    a.classList.toggle('active', a.dataset.page === pageName);
  });
  // Fetch page-specific data
  switch(pageName) {
    case 'auth': fetchAuthData(); break;
    case 'database': fetchDatabaseData(); break;
    case 'analytics': fetchAnalyticsData(); break;
    case 'settings': fetchSettingsData(); break;
  }
}

// ════════════════════════════════════
// OVERVIEW
// ════════════════════════════════════
async function fetchHealth() {
  try {
    const res = await fetch('/health');
    const data = await res.json();
    document.getElementById('server-status').innerText = 'Online';
    document.getElementById('server-status').className = 'status-online';
    document.getElementById('server-version').innerText = 'v' + (data.version || '0.1.0');
    renderServices(data.services);
  } catch {
    document.getElementById('server-status').innerText = 'Offline';
    document.getElementById('server-status').className = '';
    document.getElementById('server-status').style.color = '#ef4444';
  }
}

async function fetchAnalyticsStats() {
  try {
    const res = await fetch('/analytics/stats');
    if (!res.ok) return;
    const data = await res.json();
    const el = document.getElementById('analytics-buffer');
    if (el && data.buffer) {
      el.innerText = data.buffer.used + ' / ' + data.buffer.capacity;
    }
  } catch { /* analytics might be disabled */ }
}

function renderServices(services) {
  const container = document.getElementById('services-list');
  if (!container || !services) return;
  container.innerHTML = '';
  for (const [name] of Object.entries(services)) {
    const el = document.createElement('div');
    el.className = 'service-item';
    el.innerHTML = '<span class="service-name">' + name + '</span><span class="service-badge">Running</span>';
    container.appendChild(el);
  }
}

// ════════════════════════════════════
// AUTH PAGE
// ════════════════════════════════════
async function fetchAuthData() {
  try {
    const res = await fetch('/auth/admin/users');
    if (!res.ok) {
      document.getElementById('auth-users-list').innerHTML = '<p class="text-muted">Auth admin endpoint not available or unauthorized.</p>';
      return;
    }
    const data = await res.json();
    const users = data.users || [];
    document.getElementById('auth-total-users').innerText = users.length;
    document.getElementById('auth-signups-today').innerText = '—';
    document.getElementById('auth-sessions').innerText = '—';
    if (users.length > 0) {
      let html = '<table class="data-table"><thead><tr><th>Email</th><th>UID</th><th>Created</th></tr></thead><tbody>';
      users.slice(0, 20).forEach(u => {
        html += '<tr><td>' + (u.email||'—') + '</td><td style="font-family:monospace;font-size:0.8rem">' + (u.uid||'—').substring(0,12) + '…</td><td>' + (u.created_at||'—') + '</td></tr>';
      });
      html += '</tbody></table>';
      document.getElementById('auth-users-list').innerHTML = html;
    } else {
      document.getElementById('auth-users-list').innerHTML = '<p class="text-muted">No users registered yet.</p>';
    }
  } catch {
    document.getElementById('auth-users-list').innerHTML = '<p class="text-muted">Failed to load users.</p>';
  }
}

// ════════════════════════════════════
// DATABASE PAGE
// ════════════════════════════════════
async function fetchDatabaseData() {
  try {
    const res = await fetch('/health');
    const health = await res.json();
    if (health.stats && health.stats.database) {
      const dbStats = health.stats.database;
      document.getElementById('db-wal-seq').innerText = dbStats.wal_seq || '—';
    }
  } catch {}

  try {
    const res = await fetch('/db');
    if (!res.ok) return;
    const data = await res.json();
    const collections = data.collections || [];
    document.getElementById('db-collections-count').innerText = collections.length;
    document.getElementById('db-documents-count').innerText = '—';
    if (collections.length > 0) {
      let html = '<table class="data-table"><thead><tr><th>Collection</th><th>Actions</th></tr></thead><tbody>';
      collections.forEach(c => {
        html += '<tr><td style="font-weight:500">📁 ' + c + '</td><td class="text-muted">Browse →</td></tr>';
      });
      html += '</tbody></table>';
      document.getElementById('db-collections-list').innerHTML = html;
    } else {
      document.getElementById('db-collections-list').innerHTML = '<p class="text-muted">No collections yet. Create one via the SDK or API.</p>';
    }
  } catch {
    document.getElementById('db-collections-list').innerHTML = '<p class="text-muted">Failed to load collections.</p>';
  }
}

// ════════════════════════════════════
// ANALYTICS PAGE
// ════════════════════════════════════
async function fetchAnalyticsData() {
  try {
    const res = await fetch('/analytics/stats');
    if (!res.ok) {
      document.getElementById('analytics-counters').innerHTML = '<p class="text-muted">Analytics service not available.</p>';
      return;
    }
    const data = await res.json();
    document.getElementById('analytics-events-today').innerText = data.events_today || '0';
    document.getElementById('analytics-top-event').innerText = data.top_event || '—';
    document.getElementById('analytics-log-days').innerText = data.log_days || '—';

    if (data.counters) {
      let html = '<table class="data-table"><thead><tr><th>Metric</th><th>Count</th></tr></thead><tbody>';
      for (const [name, count] of Object.entries(data.counters)) {
        html += '<tr><td>' + name + '</td><td style="font-weight:600">' + count + '</td></tr>';
      }
      html += '</tbody></table>';
      document.getElementById('analytics-counters').innerHTML = html;
    }
  } catch {
    document.getElementById('analytics-counters').innerHTML = '<p class="text-muted">Failed to load analytics.</p>';
  }
}

// ════════════════════════════════════
// SETTINGS PAGE
// ════════════════════════════════════
async function fetchSettingsData() {
  try {
    const res = await fetch('/health');
    const data = await res.json();
    const container = document.getElementById('settings-info');
    container.innerHTML = '';

    const items = [
      { label: 'Status', value: data.status || '—' },
      { label: 'Version', value: data.version || '—' },
      { label: 'Go Version', value: data.go_version || '—' },
      { label: 'Uptime', value: data.uptime || '—' },
      { label: 'Goroutines', value: data.goroutines || '—' },
      { label: 'Memory', value: data.memory || '—' },
    ];

    items.forEach(item => {
      const div = document.createElement('div');
      div.className = 'settings-item';
      div.innerHTML = '<span class="label">' + item.label + '</span><span class="value">' + item.value + '</span>';
      container.appendChild(div);
    });
  } catch {
    document.getElementById('settings-info').innerHTML = '<p class="text-muted">Failed to load server info.</p>';
  }
}
