// ═══════════════════════════════════════════════════
// Forge Console — Interactive Engine v3
// Admin Control Center + Project Isolation
// ═══════════════════════════════════════════════════

document.addEventListener('DOMContentLoaded', () => {

  const API = window.location.origin;
  const HOSTNAME = window.location.hostname;
  const PORT = window.location.port || '8080';
  const sidebar = document.getElementById('sidebar');

  // ── Mode Detection ──
  // Port 8080 (or empty = default) = Admin Control Center
  // Any other port = Child project instance
  const IS_ADMIN = (PORT === '8080' || PORT === '');
  const IS_PROJECT = !IS_ADMIN;

  // Project port scan range
  const PORT_MIN = 8081;
  const PORT_MAX = 8100;

  // Apply mode class to body
  if (IS_ADMIN) {
    document.body.classList.add('is-admin');
    document.title = 'Forge — Admin Console';
    var ot = document.getElementById('overview-title');
    if (ot) ot.textContent = 'Admin Control Center';
    var os = document.getElementById('overview-subtitle');
    if (os) os.textContent = 'Monitor all projects, services, and infrastructure from one place.';
  } else {
    document.body.classList.add('is-project');
    document.title = 'Forge — Project :' + PORT;
    var pnd = document.getElementById('project-name-display');
    if (pnd) pnd.textContent = 'Project';
    var ppd = document.getElementById('project-port-display');
    if (ppd) ppd.textContent = ':' + PORT;
  }

  const pageTitles = {
    overview:  IS_ADMIN ? 'Admin Control Center' : 'Project Overview',
    projects:  'Projects',
    auth:      'Authentication',
    database:  'Database Browser',
    storage:   'Storage',
    analytics: 'Analytics',
    settings:  'Settings',
  };

  // ── Navigation ──
  document.querySelectorAll('.nav-item').forEach(function(link) {
    link.addEventListener('click', function(e) {
      e.preventDefault();
      var page = link.dataset.page;
      if (page) showPage(page);
    });
  });

  // ── Mobile Toggle ──
  var mobileToggle = document.getElementById('mobile-toggle');
  if (mobileToggle) mobileToggle.addEventListener('click', function() { sidebar.classList.toggle('open'); });

  // ── Init ──
  generateSnippets();
  fetchHealth();
  fetchAnalyticsStats();
  setInterval(function() { fetchHealth(); fetchAnalyticsStats(); }, 5000);

  // Hash Routing
  function handleHashChange() {
    var hash = window.location.hash.substring(1) || 'overview';
    var link = document.querySelector('.nav-item[data-page="' + hash + '"]');
    if (link) {
      showPage(hash);
    } else {
      showPage('overview');
    }
  }
  window.addEventListener('hashchange', handleHashChange);
  
  // Initialize page from hash
  handleHashChange();

  // If project mode, try to detect project name from health endpoint
  if (IS_PROJECT) {
    fetchProjectIdentity();
  }

  // ════════════════════════════════════
  // PAGE ROUTER
  // ════════════════════════════════════
  function showPage(pageName) {
    var cur = document.querySelector('.page.active-page');
    if (cur) { cur.style.animation = 'none'; cur.offsetHeight; cur.classList.remove('active-page'); }
    var target = document.getElementById('page-' + pageName);
    if (target) target.classList.add('active-page');
    document.querySelectorAll('.nav-item').forEach(function(a) { a.classList.toggle('active', a.dataset.page === pageName); });
    var t = document.getElementById('topbar-title');
    if (t) t.textContent = pageTitles[pageName] || pageName;
    sidebar.classList.remove('open');

    // Update hash without triggering hashchange event loop manually
    if (window.location.hash !== '#' + pageName) {
      history.pushState(null, null, '#' + pageName);
    }

    if (pageName === 'database') {
      document.getElementById('db-collections-view').classList.remove('hidden');
      document.getElementById('db-document-view').classList.add('hidden');
    }

    switch (pageName) {
      case 'projects':  scanProjects(); break;
      case 'auth':      fetchAuthData(); break;
      case 'database':  fetchDatabaseData(); break;
      case 'storage':   fetchStorageData(); break;
      case 'analytics': fetchAnalyticsData(); break;
      case 'settings':  fetchSettingsData(); break;
    }
  }

  // ════════════════════════════════════
  // TOAST
  // ════════════════════════════════════
  function showToast(message, type) {
    type = type || 'info';
    var c = document.getElementById('toast-container');
    if (!c) return;
    var icons = { success: '✓', error: '✕', info: 'ℹ' };
    var t = document.createElement('div');
    t.className = 'toast ' + type;
    t.innerHTML = '<span class="toast-icon">' + (icons[type] || 'ℹ') + '</span>' + escapeHtml(message);
    c.appendChild(t);
    setTimeout(function() { t.classList.add('toast-exit'); setTimeout(function() { t.remove(); }, 300); }, 3500);
  }

  // ════════════════════════════════════
  // CONNECT PANEL
  // ════════════════════════════════════
  function generateSnippets() {
    var endpoint = API;

    document.getElementById('snippet-js').innerHTML =
      '<span class="syn-cm">// Install: npm link @forge/client</span>\n' +
      '<span class="syn-kw">import</span> { <span class="syn-fn">initializeApp</span> } <span class="syn-kw">from</span> <span class="syn-str">"@forge/client"</span>;\n\n' +
      '<span class="syn-kw">const</span> <span class="syn-var">forge</span> = <span class="syn-fn">initializeApp</span>({\n' +
      '  endpoint: <span class="syn-str">"' + endpoint + '"</span>\n' +
      '});\n\n' +
      '<span class="syn-cm">// Example: Create a document</span>\n' +
      '<span class="syn-kw">await</span> <span class="syn-var">forge</span>.db.<span class="syn-fn">collection</span>(<span class="syn-str">"users"</span>).<span class="syn-fn">set</span>(<span class="syn-str">"user-1"</span>, {\n' +
      '  name: <span class="syn-str">"Alice"</span>,\n' +
      '  role: <span class="syn-str">"admin"</span>\n' +
      '});';

    document.getElementById('snippet-curl').innerHTML =
      '<span class="syn-cm"># Health check</span>\n' +
      '<span class="syn-fn">curl</span> <span class="syn-str">' + endpoint + '/health</span>\n\n' +
      '<span class="syn-cm"># Create a document</span>\n' +
      '<span class="syn-fn">curl</span> -X POST <span class="syn-str">' + endpoint + '/db/users</span> \\\n' +
      '  -H <span class="syn-str">"Content-Type: application/json"</span> \\\n' +
      '  -d <span class="syn-str">\'{"name":"Alice","role":"admin"}\'</span>\n\n' +
      '<span class="syn-cm"># Read documents</span>\n' +
      '<span class="syn-fn">curl</span> <span class="syn-str">' + endpoint + '/db/users</span>';

    document.getElementById('snippet-python').innerHTML =
      '<span class="syn-kw">import</span> requests\n\n' +
      '<span class="syn-var">BASE</span> = <span class="syn-str">"' + endpoint + '"</span>\n\n' +
      '<span class="syn-cm"># Create a document</span>\n' +
      'requests.<span class="syn-fn">post</span>(<span class="syn-kw">f</span><span class="syn-str">"{BASE}/db/users"</span>, json={\n' +
      '    <span class="syn-str">"name"</span>: <span class="syn-str">"Alice"</span>,\n' +
      '    <span class="syn-str">"role"</span>: <span class="syn-str">"admin"</span>\n' +
      '})\n\n' +
      '<span class="syn-cm"># Read documents</span>\n' +
      '<span class="syn-var">users</span> = requests.<span class="syn-fn">get</span>(<span class="syn-kw">f</span><span class="syn-str">"{BASE}/db/users"</span>).<span class="syn-fn">json</span>()';

    document.querySelectorAll('.connect-tab').forEach(function(tab) {
      tab.addEventListener('click', function() {
        document.querySelectorAll('.connect-tab').forEach(function(t) { t.classList.remove('active'); });
        document.querySelectorAll('.connect-code').forEach(function(c) { c.classList.remove('active'); });
        tab.classList.add('active');
        document.getElementById('snippet-' + tab.dataset.lang).classList.add('active');
      });
    });

    var copyBtn = document.getElementById('copy-snippet-btn');
    if (copyBtn) {
      copyBtn.addEventListener('click', function() {
        var active = document.querySelector('.connect-code.active');
        if (active) {
          navigator.clipboard.writeText(active.textContent).then(function() {
            copyBtn.textContent = '✓ Copied!';
            copyBtn.classList.add('copied');
            setTimeout(function() { copyBtn.textContent = '📋 Copy'; copyBtn.classList.remove('copied'); }, 2000);
          });
        }
      });
    }
  }

  // ════════════════════════════════════
  // PROJECT IDENTITY (Child Mode)
  // ════════════════════════════════════
  function fetchProjectIdentity() {
    // On child instances, try to extract the project name from the config
    // The systemd service description contains the project name
    // For now, derive from port: "Project on :808X"
    var pnd = document.getElementById('project-name-display');
    var ppd = document.getElementById('project-port-display');
    if (pnd) pnd.textContent = 'Instance :' + PORT;
    if (ppd) ppd.textContent = ':' + PORT;
    document.title = 'Forge — Instance :' + PORT;
  }

  // ════════════════════════════════════
  // PROJECTS (Admin Mode — Port Scanning)
  // ════════════════════════════════════
  var projectsCache = [];

  var refreshBtn = document.getElementById('projects-refresh-btn');
  if (refreshBtn) refreshBtn.addEventListener('click', function() { scanProjects(); });

  async function scanProjects() {
    var grid = document.getElementById('projects-grid');
    if (!grid) return;

    // Show scanning state
    grid.innerHTML = '';
    var scanCount = PORT_MAX - PORT_MIN + 1;
    for (var p = PORT_MIN; p <= PORT_MAX; p++) {
      var placeholder = document.createElement('div');
      placeholder.className = 'project-card';
      placeholder.id = 'pcard-' + p;
      placeholder.innerHTML =
        '<div class="project-card-header">' +
          '<div class="project-card-title">Port ' + p + '</div>' +
          '<div class="project-status-dot scanning"></div>' +
        '</div>' +
        '<div class="project-card-meta">' +
          '<div class="project-meta-row"><span class="meta-label">Status</span><span class="meta-value">Scanning...</span></div>' +
        '</div>';
      placeholder.style.display = 'none';
      grid.appendChild(placeholder);
    }

    showToast('Scanning ports ' + PORT_MIN + '–' + PORT_MAX + '...', 'info');
    logActivity('SCAN', '/ports/' + PORT_MIN + '-' + PORT_MAX);

    var activeCount = 0;
    var allHealthy = true;
    var promises = [];

    for (var port = PORT_MIN; port <= PORT_MAX; port++) {
      (function(p) {
        var url = window.location.protocol + '//' + HOSTNAME + ':' + p + '/health';
        var promise = fetchWithTimeout(url, 2000)
          .then(function(res) { return res.json(); })
          .then(function(data) {
            activeCount++;
            var card = document.getElementById('pcard-' + p);
            if (card) {
              card.style.display = '';
              var name = data.project_name || 'Instance ' + p;
              var version = data.version || '—';
              var services = data.services ? Object.keys(data.services).length : 0;
              card.innerHTML =
                '<div class="project-card-header">' +
                  '<div class="project-card-title">' + escapeHtml(name) + '</div>' +
                  '<div class="project-status-dot online"></div>' +
                '</div>' +
                '<div class="project-card-meta">' +
                  '<div class="project-meta-row"><span class="meta-label">Port</span><span class="meta-value">' + p + '</span></div>' +
                  '<div class="project-meta-row"><span class="meta-label">Version</span><span class="meta-value">v' + escapeHtml(version) + '</span></div>' +
                  '<div class="project-meta-row"><span class="meta-label">Services</span><span class="meta-value">' + services + ' active</span></div>' +
                '</div>' +
                '<div class="project-card-footer">' +
                  '<div class="project-card-footer-buttons">' +
                    '<a class="project-open-btn" href="' + window.location.protocol + '//' + HOSTNAME + ':' + p + '/dashboard" target="_blank">Open Console →</a>' +
                    '<button class="project-delete-btn" data-port="' + p + '" data-name="' + escapeHtml(name) + '">Delete</button>' +
                  '</div>' +
                  '<span class="project-card-services">:' + p + '</span>' +
                '</div>';
            }
          })
          .catch(function() {
            // Port not responsive — hide card
            var card = document.getElementById('pcard-' + p);
            if (card) card.style.display = 'none';
          });
        promises.push(promise);
      })(port);
    }

    Promise.all(promises).then(function() {
      document.getElementById('projects-active-count').textContent = activeCount;
      document.getElementById('projects-health-status').textContent = activeCount > 0 ? (allHealthy ? 'Yes ✓' : 'Issues') : '—';

      // Bind delete buttons
      document.querySelectorAll('.project-delete-btn').forEach(function(btn) {
        btn.addEventListener('click', function() {
          showDeleteModal(this.dataset.name, this.dataset.port);
        });
      });

      if (activeCount === 0) {
        grid.innerHTML =
          '<div class="empty-state" style="grid-column:1/-1">' +
            '<div class="empty-icon">📦</div>' +
            '<h3>No Projects Running</h3>' +
            '<p>Deploy your first project to see it here.</p>' +
            '<pre class="empty-code">./run-my-backend "My App"</pre>' +
          '</div>';
      }

      showToast('Found ' + activeCount + ' active project' + (activeCount !== 1 ? 's' : ''), activeCount > 0 ? 'success' : 'info');
    });
  }

  function fetchWithTimeout(url, ms) {
    var controller = new AbortController();
    var timer = setTimeout(function() { controller.abort(); }, ms);
    return fetch(url, { signal: controller.signal }).then(function(res) {
      clearTimeout(timer);
      return res;
    });
  }

  // ════════════════════════════════════
  // DESTRUCTION PROTOCOL
  // ════════════════════════════════════
  function showDeleteModal(name, port) {
    var modal = document.getElementById('delete-modal');
    if (!modal) return;
    
    document.getElementById('delete-modal-project-name').textContent = name;
    document.getElementById('delete-modal-project-port').textContent = port;
    
    var slug = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
    var code = 'sudo systemctl stop forge-' + slug + '\\n' +
               'sudo systemctl disable forge-' + slug + '\\n' +
               'sudo rm -rf /opt/forge/projects/' + slug + '\\n' +
               'sudo rm -f /etc/systemd/system/forge-' + slug + '.service\\n' +
               'sudo systemctl daemon-reload';
               
    document.getElementById('delete-modal-code').innerHTML = code;
    
    var copyBtn = document.getElementById('delete-modal-copy-btn');
    var realCodeText = code.replace(/\\n/g, '\n');
    copyBtn.onclick = function() {
      navigator.clipboard.writeText(realCodeText).then(function() {
        copyBtn.textContent = '✓ Copied to Clipboard';
        copyBtn.classList.add('copied');
        setTimeout(function() { copyBtn.textContent = '📋 Copy Command'; copyBtn.classList.remove('copied'); }, 2000);
      });
    };
    
    modal.classList.add('active');
  }
  
  var closeDelBtn = document.getElementById('close-delete-modal');
  if (closeDelBtn) {
    closeDelBtn.addEventListener('click', function() {
      document.getElementById('delete-modal').classList.remove('active');
    });
  }

  // ════════════════════════════════════
  // OVERVIEW
  // ════════════════════════════════════
  var activityLog = [];

  async function fetchHealth() {
    try {
      var res = await fetch(API + '/health');
      var data = await res.json();

      document.getElementById('server-status').textContent = 'Online';
      document.getElementById('server-status').className = 'status-online';
      document.getElementById('server-version').textContent = 'v' + (data.version || '0.1.0');

      var badge = document.getElementById('topbar-badge');
      if (badge) { badge.textContent = '● Online'; badge.style.color = ''; badge.style.borderColor = ''; badge.style.background = ''; }

      var dot = document.getElementById('pulse-dot');
      var ss = document.getElementById('sidebar-status');
      if (dot) dot.style.background = 'var(--accent-emerald)';
      if (ss) ss.textContent = 'Connected';

      renderServices(data.services);
      logActivity('GET', '/health');

      // On project mode, try to get project name from health data
      if (IS_PROJECT && data.project_name) {
        var pnd = document.getElementById('project-name-display');
        if (pnd) pnd.textContent = data.project_name;
        document.title = 'Forge — ' + data.project_name;
      }
    } catch(e) {
      document.getElementById('server-status').textContent = 'Offline';
      document.getElementById('server-status').className = 'status-offline';
      var badge = document.getElementById('topbar-badge');
      if (badge) { badge.textContent = '● Offline'; badge.style.color = 'var(--accent-rose)'; badge.style.borderColor = 'rgba(251,113,133,0.2)'; badge.style.background = 'rgba(251,113,133,0.1)'; }
      var dot = document.getElementById('pulse-dot');
      var ss = document.getElementById('sidebar-status');
      if (dot) dot.style.background = 'var(--accent-rose)';
      if (ss) ss.textContent = 'Disconnected';
    }
  }

  async function fetchAnalyticsStats() {
    try {
      var res = await fetch(API + '/analytics/stats');
      if (!res.ok) return;
      var data = await res.json();
      var el = document.getElementById('analytics-buffer');
      if (el && data.buffer) el.textContent = data.buffer.used + ' / ' + data.buffer.capacity;
    } catch(e) {}
  }

  function renderServices(services) {
    var c = document.getElementById('services-list');
    if (!c || !services) return;
    c.innerHTML = '';
    for (var name in services) {
      var chip = document.createElement('div');
      chip.className = 'service-chip';
      chip.innerHTML = '<div class="service-dot"></div><span class="service-name">' + escapeHtml(name) + '</span>';
      c.appendChild(chip);
    }
  }

  function logActivity(method, path) {
    activityLog.unshift({ method: method, path: path, time: new Date() });
    if (activityLog.length > 8) activityLog = activityLog.slice(0, 8);
    renderActivity();
  }

  function renderActivity() {
    var c = document.getElementById('activity-feed');
    if (!c) return;
    if (activityLog.length === 0) {
      c.innerHTML = '<div class="empty-state"><p>No activity yet.</p></div>';
      return;
    }
    c.innerHTML = '';
    activityLog.forEach(function(a) {
      var item = document.createElement('div');
      item.className = 'activity-item';
      var t = a.time;
      var ts = t.getHours().toString().padStart(2,'0') + ':' + t.getMinutes().toString().padStart(2,'0') + ':' + t.getSeconds().toString().padStart(2,'0');
      var methodClass = 'method-' + a.method;
      if (a.method === 'SCAN') methodClass = 'method-POST';
      item.innerHTML =
        '<span class="activity-method ' + methodClass + '">' + a.method + '</span>' +
        '<span class="activity-path">' + escapeHtml(a.path) + '</span>' +
        '<span class="activity-time">' + ts + '</span>';
      c.appendChild(item);
    });
  }

  // ════════════════════════════════════
  // AUTH
  // ════════════════════════════════════
  async function fetchAuthData() {
    logActivity('GET', '/auth/admin/users');
    try {
      var res = await fetch(API + '/auth/admin/users');
      if (!res.ok) {
        document.getElementById('auth-users-list').innerHTML = emptyState('🔒', 'Auth Admin Required', 'The admin endpoint requires authorization.', 'curl -X POST ' + API + '/auth/signup \\\n  -H "Content-Type: application/json" \\\n  -d \'{"email":"you@mail.com","password":"secret"}\'');
        return;
      }
      var data = await res.json();
      var users = data.users || [];
      document.getElementById('auth-total-users').textContent = users.length;
      document.getElementById('auth-signups-today').textContent = '—';
      document.getElementById('auth-sessions').textContent = '—';

      if (users.length > 0) {
        var html = '<table class="data-table"><thead><tr><th>Email</th><th>UID</th><th>Created</th></tr></thead><tbody>';
        users.slice(0, 20).forEach(function(u) {
          var uid = (u.uid || '—').substring(0, 16);
          html += '<tr><td>' + escapeHtml(u.email || '—') + '</td><td class="mono">' + escapeHtml(uid) + '…</td><td class="text-muted">' + escapeHtml(u.created_at || '—') + '</td></tr>';
        });
        html += '</tbody></table>';
        document.getElementById('auth-users-list').innerHTML = html;
        showToast('Loaded ' + users.length + ' users', 'success');
      } else {
        document.getElementById('auth-users-list').innerHTML = emptyState('👤', 'No Users Yet', 'Create your first user with the SDK or REST API.', 'curl -X POST ' + API + '/auth/signup \\\n  -H "Content-Type: application/json" \\\n  -d \'{"email":"you@mail.com","password":"secret"}\'');
      }
    } catch(e) {
      document.getElementById('auth-users-list').innerHTML = emptyState('⚠️', 'Connection Error', 'Could not reach the auth service.');
      showToast('Failed to load auth data', 'error');
    }
  }

  // ════════════════════════════════════
  // DATABASE
  // ════════════════════════════════════
  async function fetchDatabaseData() {
    logActivity('GET', '/db/collections');
    try {
      var res = await fetch(API + '/db/collections');
      if (!res.ok) return;
      var data = await res.json();
      var collections = data.collections || [];

      var totalDocs = 0;
      collections.forEach(function(c) { totalDocs += (c.count || 0); });
      document.getElementById('db-collections-count').textContent = collections.length;
      document.getElementById('db-documents-count').textContent = totalDocs;
      document.getElementById('db-wal-seq').textContent = 'Active';

      if (collections.length > 0) {
        var html = '<table class="data-table"><thead><tr><th>Collection</th><th>Documents</th><th>Action</th></tr></thead><tbody>';
        collections.forEach(function(c) {
          var name = c.name || c;
          var count = c.count || '—';
          html += '<tr class="clickable-row" data-collection="' + escapeHtml(name) + '">' +
            '<td style="font-weight:500">📁 ' + escapeHtml(name) + '</td>' +
            '<td class="mono">' + count + '</td>' +
            '<td style="color:var(--accent-indigo);font-size:0.78rem;font-weight:500">Browse →</td></tr>';
        });
        html += '</tbody></table>';
        document.getElementById('db-collections-list').innerHTML = html;

        document.querySelectorAll('.clickable-row[data-collection]').forEach(function(row) {
          row.addEventListener('click', function() { browseCollection(row.dataset.collection); });
        });
      } else {
        document.getElementById('db-collections-list').innerHTML = emptyState('📂', 'No Collections Yet', 'Create your first collection by saving a document.', 'curl -X POST ' + API + '/db/my_collection \\\n  -H "Content-Type: application/json" \\\n  -d \'{"name":"Hello","status":"World"}\'');
      }
    } catch(e) {
      document.getElementById('db-collections-list').innerHTML = emptyState('⚠️', 'Connection Error', 'Could not reach the database service.');
      showToast('Failed to load database', 'error');
    }
  }

  async function browseCollection(name) {
    logActivity('GET', '/db/' + name);
    try {
      var res = await fetch(API + '/db/' + encodeURIComponent(name) + '?limit=50');
      if (!res.ok) return;
      var data = await res.json();
      var docs = data.documents || [];

      document.getElementById('db-collections-view').classList.add('hidden');
      document.getElementById('db-document-view').classList.remove('hidden');
      document.getElementById('db-doc-title').textContent = '📁 ' + name;
      document.getElementById('db-doc-count').textContent = docs.length + ' doc' + (docs.length !== 1 ? 's' : '');

      var container = document.getElementById('db-documents-list');
      container.innerHTML = '';

      if (docs.length === 0) {
        container.innerHTML = emptyState('📄', 'Empty Collection', 'This collection exists but has no documents yet.');
        return;
      }

      docs.forEach(function(doc) {
        var card = document.createElement('div');
        card.className = 'doc-card';
        var id = doc._id || 'unknown';
        var created = doc._created_at ? new Date(doc._created_at).toLocaleString() : '';

        var headerHtml = '<div class="doc-card-header"><span class="doc-id">' + escapeHtml(id) + '</span><span class="doc-meta">' + escapeHtml(created) + '</span></div>';
        var bodyHtml = '<div class="doc-card-body">';
        for (var key in doc) {
          var val = doc[key];
          var valClass = 'string';
          if (typeof val === 'number') valClass = 'number';
          else if (typeof val === 'boolean') valClass = 'bool';
          if (key.startsWith('_')) valClass = 'meta';
          var display = typeof val === 'object' ? JSON.stringify(val) : String(val);
          bodyHtml += '<div class="doc-field"><span class="doc-field-key">' + escapeHtml(key) + '</span><span class="doc-field-value ' + valClass + '">' + escapeHtml(display) + '</span></div>';
        }
        bodyHtml += '</div>';
        card.innerHTML = headerHtml + bodyHtml;
        card.querySelector('.doc-card-header').addEventListener('click', function() { card.classList.toggle('expanded'); });
        container.appendChild(card);
      });

      var first = container.querySelector('.doc-card');
      if (first) first.classList.add('expanded');
      showToast('Loaded ' + docs.length + ' documents from ' + name, 'success');
    } catch(e) {
      showToast('Failed to load documents', 'error');
    }
  }

  var backBtn = document.getElementById('db-back-btn');
  if (backBtn) {
    backBtn.addEventListener('click', function() {
      document.getElementById('db-document-view').classList.add('hidden');
      document.getElementById('db-collections-view').classList.remove('hidden');
    });
  }

  // ════════════════════════════════════
  // STORAGE
  // ════════════════════════════════════
  async function fetchStorageData() {
    logActivity('GET', '/storage/list');
    try {
      var res = await fetch(API + '/storage/list');
      if (!res.ok) {
        document.getElementById('storage-files-list').innerHTML = emptyState('📁', 'No Files Yet', 'Upload your first file using the zone above or via the API.', 'curl -X POST ' + API + '/storage/upload/photos/cat.jpg \\\n  -H "Content-Type: image/jpeg" \\\n  --data-binary @cat.jpg');
        document.getElementById('storage-file-count').textContent = '0';
        document.getElementById('storage-size').textContent = '0 B';
        return;
      }
      var data = await res.json();
      var files = data.files || [];
      document.getElementById('storage-file-count').textContent = files.length;
      var totalSize = 0;
      files.forEach(function(f) { totalSize += (f.size || 0); });
      document.getElementById('storage-size').textContent = formatBytes(totalSize);

      if (files.length > 0) {
        var html = '<table class="data-table"><thead><tr><th>File</th><th>Size</th><th>Type</th><th>Action</th></tr></thead><tbody>';
        files.forEach(function(f) {
          var icon = getFileIcon(f.name || f.path || '');
          html += '<tr><td style="font-weight:500">' + icon + ' ' + escapeHtml(f.name || f.path || '—') + '</td><td class="mono">' + formatBytes(f.size || 0) + '</td><td class="text-muted">' + escapeHtml(f.content_type || '—') + '</td><td><a href="' + API + '/storage/object/' + encodeURIComponent(f.path || f.name || '') + '" target="_blank" style="color:var(--accent-indigo);font-size:0.78rem;font-weight:500;text-decoration:none">Download ↓</a></td></tr>';
        });
        html += '</tbody></table>';
        document.getElementById('storage-files-list').innerHTML = html;
        showToast(files.length + ' file(s) loaded', 'success');
      } else {
        document.getElementById('storage-files-list').innerHTML = emptyState('📁', 'No Files Yet', 'Upload your first file using the zone above or via the API.', 'curl -X POST ' + API + '/storage/upload/photos/cat.jpg \\\n  -H "Content-Type: image/jpeg" \\\n  --data-binary @cat.jpg');
      }
    } catch(e) {
      document.getElementById('storage-files-list').innerHTML = emptyState('⚠️', 'Connection Error', 'Could not reach the storage service.');
      showToast('Failed to load files', 'error');
    }
  }

  var uploadZone = document.getElementById('upload-zone');
  var uploadInput = document.getElementById('upload-input');
  if (uploadZone && uploadInput) {
    uploadZone.addEventListener('click', function() { uploadInput.click(); });
    uploadZone.addEventListener('dragover', function(e) { e.preventDefault(); uploadZone.classList.add('dragover'); });
    uploadZone.addEventListener('dragleave', function() { uploadZone.classList.remove('dragover'); });
    uploadZone.addEventListener('drop', function(e) { e.preventDefault(); uploadZone.classList.remove('dragover'); handleFiles(e.dataTransfer.files); });
    uploadInput.addEventListener('change', function() { handleFiles(uploadInput.files); });
  }

  async function handleFiles(fileList) {
    for (var i = 0; i < fileList.length; i++) {
      var file = fileList[i];
      try {
        var path = 'uploads/' + file.name;
        logActivity('POST', '/storage/upload/' + path);
        var res = await fetch(API + '/storage/upload/' + path, {
          method: 'POST',
          headers: { 'Content-Type': file.type || 'application/octet-stream' },
          body: file,
        });
        if (res.ok) showToast('Uploaded: ' + file.name, 'success');
        else showToast('Upload failed: ' + file.name, 'error');
      } catch(e) {
        showToast('Upload error: ' + file.name, 'error');
      }
    }
    fetchStorageData();
  }

  // ════════════════════════════════════
  // ANALYTICS
  // ════════════════════════════════════
  async function fetchAnalyticsData() {
    logActivity('GET', '/analytics/stats');
    try {
      var res = await fetch(API + '/analytics/stats');
      if (!res.ok) {
        document.getElementById('analytics-counters').innerHTML = emptyState('📊', 'No Events Yet', 'Start tracking events with the SDK or API.', 'curl -X POST ' + API + '/analytics/track \\\n  -H "Content-Type: application/json" \\\n  -d \'{"name":"page_view","properties":{"page":"/home"}}\'');
        return;
      }
      var data = await res.json();
      document.getElementById('analytics-events-today').textContent = data.events_today || '0';
      document.getElementById('analytics-top-event').textContent = data.top_event || '—';
      document.getElementById('analytics-log-days').textContent = data.log_days || '—';

      if (data.counters && Object.keys(data.counters).length > 0) {
        var html = '<table class="data-table"><thead><tr><th>Event</th><th>Count</th></tr></thead><tbody>';
        for (var name in data.counters) {
          html += '<tr><td>' + escapeHtml(name) + '</td><td style="font-weight:600;color:var(--accent-cyan)">' + data.counters[name] + '</td></tr>';
        }
        html += '</tbody></table>';
        document.getElementById('analytics-counters').innerHTML = html;
      } else {
        document.getElementById('analytics-counters').innerHTML = emptyState('📊', 'No Events Yet', 'Start tracking events with the SDK or API.', 'curl -X POST ' + API + '/analytics/track \\\n  -H "Content-Type: application/json" \\\n  -d \'{"name":"page_view","properties":{"page":"/home"}}\'');
      }
    } catch(e) {
      document.getElementById('analytics-counters').innerHTML = emptyState('⚠️', 'Connection Error', 'Could not reach the analytics service.');
      showToast('Failed to load analytics', 'error');
    }
  }

  // ════════════════════════════════════
  // SETTINGS
  // ════════════════════════════════════
  async function fetchSettingsData() {
    logActivity('GET', '/health');
    try {
      var res = await fetch(API + '/health');
      var data = await res.json();
      var c = document.getElementById('settings-info');
      c.innerHTML = '';
      var items = [
        { label: 'Mode', value: IS_ADMIN ? 'Admin Control Center' : 'Project Instance' },
        { label: 'Status', value: data.status || '—' },
        { label: 'Version', value: data.version || '—' },
        { label: 'API Endpoint', value: API },
        { label: 'Port', value: ':' + PORT },
        { label: 'Services', value: data.services ? Object.keys(data.services).length + ' active' : '—' },
        { label: 'Timestamp', value: data.timestamp || '—' },
        { label: 'Engine', value: 'DynamicDB / LSM + MVCC' },
      ];
      items.forEach(function(item) {
        var div = document.createElement('div');
        div.className = 'settings-item';
        div.innerHTML = '<span class="label">' + escapeHtml(item.label) + '</span><span class="value">' + escapeHtml(item.value) + '</span>';
        c.appendChild(div);
      });
    } catch(e) {
      document.getElementById('settings-info').innerHTML = emptyState('⚠️', 'Connection Error', 'Could not reach the server.');
      showToast('Failed to load settings', 'error');
    }
  }

  // ════════════════════════════════════
  // UTILITIES
  // ════════════════════════════════════
  function escapeHtml(str) {
    if (str === null || str === undefined) return '—';
    return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function formatBytes(bytes) {
    if (!bytes || bytes === 0) return '0 B';
    var k = 1024;
    var sizes = ['B', 'KB', 'MB', 'GB'];
    var i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  }

  function getFileIcon(name) {
    var ext = (name.split('.').pop() || '').toLowerCase();
    var icons = { jpg:'🖼️', jpeg:'🖼️', png:'🖼️', gif:'🖼️', webp:'🖼️', svg:'🖼️', pdf:'📕', mp4:'🎬', mp3:'🎵', zip:'📦', json:'📋', csv:'📊', txt:'📝', html:'🌐', css:'🎨', js:'⚙️' };
    return icons[ext] || '📄';
  }

  function emptyState(icon, title, desc, code) {
    var html = '<div class="empty-state"><div class="empty-icon">' + icon + '</div><h3>' + escapeHtml(title) + '</h3><p>' + escapeHtml(desc) + '</p>';
    if (code) html += '<pre class="empty-code">' + escapeHtml(code) + '</pre>';
    html += '</div>';
    return html;
  }

});
