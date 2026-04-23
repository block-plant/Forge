// ═══════════════════════════════════════════════════
// Forge Console — Interactive Engine v4
// Admin Control Center + Project Isolation
// Fixed: page sync, scroll animations, device compat
// ═══════════════════════════════════════════════════

document.addEventListener('DOMContentLoaded', () => {

  const API = window.location.origin;
  const HOSTNAME = window.location.hostname;
  const PORT = window.location.port || '8080';
  const sidebar = document.getElementById('sidebar');
  const overlay = document.getElementById('sidebar-overlay');

  // ── Theme Management ──
  const themeToggle = document.getElementById('theme-toggle');
  const iconSun = document.getElementById('theme-icon-sun');
  const iconMoon = document.getElementById('theme-icon-moon');
  
  function applyTheme(isDark) {
    if (isDark) {
      document.documentElement.setAttribute('data-theme', 'dark');
      if (iconSun) iconSun.classList.remove('hidden');
      if (iconMoon) iconMoon.classList.add('hidden');
    } else {
      document.documentElement.removeAttribute('data-theme');
      if (iconSun) iconSun.classList.add('hidden');
      if (iconMoon) iconMoon.classList.remove('hidden');
    }
  }
  
  let currentDark = localStorage.getItem('theme') !== 'light';
  applyTheme(currentDark);
  
  if (themeToggle) {
    themeToggle.addEventListener('click', () => {
      currentDark = !currentDark;
      localStorage.setItem('theme', currentDark ? 'dark' : 'light');
      applyTheme(currentDark);
    });
  }

  // ── Mode Detection ──
  const IS_ADMIN = (PORT === '8080' || PORT === '');
  const IS_PROJECT = !IS_ADMIN;
  const PORT_MIN = 8081;
  const PORT_MAX = 8100;

  function getForgeToken() {
    return localStorage.getItem('forge_token') || localStorage.getItem('token') || localStorage.getItem('auth_token') || '';
  }

  function setForgeToken(token) {
    if (!token) return;
    localStorage.setItem('forge_token', token);
    localStorage.setItem('token', token);
    localStorage.setItem('auth_token', token);
  }

  function renderAuthSigninPrompt(message) {
    var info = message || 'Sign in to view authentication data.';
    var html =
      '<div class="empty-state" style="max-width:520px;margin:0 auto">' +
        '<div class="empty-icon">🔐</div>' +
        '<h3>Authentication Required</h3>' +
        '<p>' + escapeHtml(info) + '</p>' +
        '<div style="margin-top:0.85rem;text-align:left;display:flex;flex-direction:column;gap:0.55rem">' +
          '<input id="auth-signin-email" class="modal-input" type="email" placeholder="you@email.com" style="width:100%">' +
          '<input id="auth-signin-password" class="modal-input" type="password" placeholder="password" style="width:100%">' +
          '<button id="auth-signin-btn" class="primary-btn" style="width:100%">Sign In</button>' +
        '</div>' +
      '</div>';
    document.getElementById('auth-users-list').innerHTML = html;

    var btn = document.getElementById('auth-signin-btn');
    if (btn) {
      btn.addEventListener('click', async function() {
        var emailEl = document.getElementById('auth-signin-email');
        var passEl = document.getElementById('auth-signin-password');
        var email = emailEl ? String(emailEl.value || '').trim() : '';
        var password = passEl ? String(passEl.value || '') : '';
        if (!email || !password) {
          showToast('Email and password are required', 'error');
          return;
        }
        btn.disabled = true;
        btn.textContent = 'Signing In...';
        try {
          var signinRes = await fetch(API + '/auth/signin', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email: email, password: password }),
          });
          var signinData = await signinRes.json().catch(function() { return {}; });
          if (!signinRes.ok) {
            throw new Error(signinData.message || ('Sign in failed (' + signinRes.status + ')'));
          }
          var token = (signinData.tokens && (signinData.tokens.access_token || signinData.tokens.token)) || '';
          if (!token) throw new Error('No access token returned');
          setForgeToken(token);
          showToast('Signed in successfully', 'success');
          fetchAuthData();
        } catch (err) {
          showToast('Sign in failed: ' + (err && err.message ? err.message : 'Unknown error'), 'error');
          btn.disabled = false;
          btn.textContent = 'Sign In';
        }
      });
    }
  }

  async function apiFetch(url, options) {
    options = options || {};
    var headers = new Headers(options.headers || {});
    var token = getForgeToken();
    if (token) headers.set('Authorization', 'Bearer ' + token);
    return fetch(url, Object.assign({}, options, { headers: headers }));
  }

  function projectApiBase(port) {
    return window.location.protocol + '//' + HOSTNAME + ':' + String(port || '').trim();
  }

  async function requestProjectPurge(endpoint, scopes) {
    var scopeList = Array.isArray(scopes) ? scopes : [scopes];
    var confirmed = window.prompt('Type DESTROY to confirm this destructive action:');
    if (confirmed !== 'DESTROY') {
      showToast('Cancelled. Confirmation did not match DESTROY.', 'info');
      return false;
    }
    var res = await apiFetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm: 'DESTROY', scopes: scopeList }),
    });
    var text = await res.text();
    if (!res.ok) {
      throw new Error(text || ('Purge failed (' + res.status + ')'));
    }
    return true;
  }

  var adminSummaryCache = null;
  var adminSummaryCacheAt = 0;

  async function fetchAdminSummary(force) {
    if (!IS_ADMIN) return null;
    var now = Date.now();
    if (!force && adminSummaryCache && (now - adminSummaryCacheAt) < 5000) {
      return adminSummaryCache;
    }
    var res = await apiFetch(API + '/admin/projects/summary');
    if (!res.ok) throw new Error(await res.text());
    var data = await res.json();
    adminSummaryCache = data;
    adminSummaryCacheAt = now;
    return data;
  }

  // Apply mode class
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
    guide:     'Guide & FAQ',
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

  // Topbar toggles
  const mobileToggle = document.getElementById('mobile-toggle');
  if (mobileToggle) {
    mobileToggle.addEventListener('click', () => {
      sidebar.classList.toggle('open');
      if (overlay) overlay.classList.toggle('active', sidebar.classList.contains('open'));
    });
  }
  const desktopToggle = document.getElementById('desktop-toggle');
  if (desktopToggle) {
    desktopToggle.addEventListener('click', () => {
      document.body.classList.toggle('sidebar-collapsed');
      sidebar.classList.toggle('collapsed');
    });
  }

  // ── Mobile Toggle + Overlay ──
  if (overlay) {
    overlay.addEventListener('click', function() {
      sidebar.classList.remove('open');
      overlay.classList.remove('active');
    });
  }

  // ── Init — fetch data immediately ──
  generateSnippets();
  fetchHealth();
  fetchAnalyticsStats();
  setInterval(function() { fetchHealth(); fetchAnalyticsStats(); }, 8000);

  // Hash Routing
  function handleHashChange() {
    var hash = window.location.hash.substring(1) || 'overview';
    showPage(hash);
  }
  window.addEventListener('hashchange', handleHashChange);
  handleHashChange();

  // If project mode, detect identity
  if (IS_PROJECT) fetchProjectIdentity();

  // ════════════════════════════════════
  // PAGE ROUTER (instant, no delays)
  // ════════════════════════════════════
  function showPage(pageName) {
    // Validate page exists
    var target = document.getElementById('page-' + pageName);
    if (!target) { pageName = 'overview'; target = document.getElementById('page-overview'); }

    // Instant page switch — remove old, show new
    var cur = document.querySelector('.page.active-page');
    if (cur && cur !== target) {
      cur.classList.remove('active-page');
      cur.style.animation = 'none';
    }
    target.style.animation = '';
    target.classList.add('active-page');

    // Update nav
    document.querySelectorAll('.nav-item').forEach(function(a) { a.classList.toggle('active', a.dataset.page === pageName); });
    var t = document.getElementById('topbar-title');
    if (t) t.textContent = pageTitles[pageName] || pageName;

    // Close mobile sidebar
    sidebar.classList.remove('open');
    if (overlay) overlay.classList.remove('active');

    // Update hash
    if (window.location.hash !== '#' + pageName) {
      history.pushState(null, null, '#' + pageName);
    }

    // Reset database view
    if (pageName === 'database') {
      document.getElementById('db-collections-view').classList.remove('hidden');
      document.getElementById('db-document-view').classList.add('hidden');
    }

    // Setup guide FAQ accordion
    if (pageName === 'guide') initFaqAccordion();

    // Fetch data for the page
    switch (pageName) {
      case 'projects':  scanProjects(); break;
      case 'auth':      fetchAuthData(); break;
      case 'database':  fetchDatabaseData(); break;
      case 'storage':   fetchStorageData(); break;
      case 'analytics': fetchAnalyticsData(); break;
      case 'settings':  fetchSettingsData(); break;
    }

    // Scroll to top of page content
    var mainContent = document.querySelector('.main-content');
    if (mainContent) mainContent.scrollTop = 0;
  }

  // ════════════════════════════════════
  // FAQ ACCORDION
  // ════════════════════════════════════
  var faqInitialized = false;
  function initFaqAccordion() {
    if (faqInitialized) return;
    faqInitialized = true;
    document.querySelectorAll('.faq-question').forEach(function(q) {
      q.addEventListener('click', function() {
        var item = q.parentElement;
        // Close others
        document.querySelectorAll('.faq-item.open').forEach(function(other) {
          if (other !== item) other.classList.remove('open');
        });
        item.classList.toggle('open');
      });
    });
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
    setTimeout(function() { t.classList.add('toast-exit'); setTimeout(function() { t.remove(); }, 300); }, 3000);
  }

  // ════════════════════════════════════
  // CONNECT PANEL
  // ════════════════════════════════════
  function generateSnippets() {
    var endpoint = API;

    document.getElementById('snippet-js').innerHTML =
      '<span class="syn-cm">// Connect from any JavaScript app</span>\n' +
      '<span class="syn-kw">const</span> <span class="syn-var">FORGE</span> = <span class="syn-str">"' + endpoint + '"</span>;\n\n' +
      '<span class="syn-cm">// Create a document</span>\n' +
      '<span class="syn-kw">await</span> <span class="syn-fn">fetch</span>(<span class="syn-str">`${FORGE}/db/users`</span>, {\n' +
      '  method: <span class="syn-str">"POST"</span>,\n' +
      '  headers: { <span class="syn-str">"Content-Type"</span>: <span class="syn-str">"application/json"</span> },\n' +
      '  body: <span class="syn-fn">JSON.stringify</span>({ name: <span class="syn-str">"Alice"</span>, role: <span class="syn-str">"admin"</span> })\n' +
      '});\n\n' +
      '<span class="syn-cm">// Read documents</span>\n' +
      '<span class="syn-kw">const</span> <span class="syn-var">res</span> = <span class="syn-kw">await</span> <span class="syn-fn">fetch</span>(<span class="syn-str">`${FORGE}/db/users`</span>);\n' +
      '<span class="syn-kw">const</span> <span class="syn-var">data</span> = <span class="syn-kw">await</span> <span class="syn-var">res</span>.<span class="syn-fn">json</span>();';

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
    var pnd = document.getElementById('project-name-display');
    var ppd = document.getElementById('project-port-display');
    if (pnd) pnd.textContent = 'Instance :' + PORT;
    if (ppd) ppd.textContent = ':' + PORT;
    document.title = 'Forge — Instance :' + PORT;
  }

  // ════════════════════════════════════
  // PROJECTS (Admin Mode — Port Scanning)
  // ════════════════════════════════════
  var refreshBtn = document.getElementById('projects-refresh-btn');
  if (refreshBtn) refreshBtn.addEventListener('click', function() { scanProjects(); });

  async function scanProjects(retryCount = 0) {
    var grid = document.getElementById('projects-grid');
    if (!grid) return;

    if (retryCount === 0) {
      grid.innerHTML = '<div style="text-align:center;padding:1.5rem;color:var(--text-tertiary);font-size:0.82rem">Loading projects from engine...</div>';
    }
    logActivity('GET', '/admin/projects' + (retryCount > 0 ? ' (Retry ' + retryCount + ')' : ''));

    try {
      var summary = await fetchAdminSummary(retryCount > 0);
      var projects = (summary && summary.projects) || [];
      var totals = (summary && summary.totals) || {};
      document.getElementById('projects-active-count').textContent = String(totals.projects ?? projects.length);
      document.getElementById('projects-health-status').textContent = (totals.offline || 0) === 0 ? 'Yes ✓' : 'No';

      grid.innerHTML = '';
      if (projects.length === 0) {
        grid.innerHTML =
          '<div class="empty-state" style="grid-column:1/-1">' +
            '<div class="empty-icon">📦</div>' +
            '<h3>No Projects Running</h3>' +
            '<p>Click "Deploy" to provision your first project.</p>' +
          '</div>';
      } else {
        projects.sort(function(a, b) { return a.port - b.port; });
        projects.forEach(function(p) {
          var card = document.createElement('div');
          card.className = 'project-card';
          card.innerHTML =
            '<div class="project-card-header">' +
              '<div class="project-card-title">' + escapeHtml(p.name) + '</div>' +
              '<div class="project-status-dot ' + (p.health === 'online' ? 'online' : '') + '"></div>' +
            '</div>' +
            '<div class="project-card-meta">' +
              '<div class="project-meta-row"><span class="meta-label">Port</span><span class="meta-value">' + p.port + '</span></div>' +
              '<div class="project-meta-row"><span class="meta-label">Slug</span><span class="meta-value">' + escapeHtml(p.slug) + '</span></div>' +
              '<div class="project-meta-row"><span class="meta-label">Health</span><span class="meta-value">' + escapeHtml((p.health || 'unknown').toUpperCase()) + '</span></div>' +
            '</div>' +
            '<div class="project-card-footer">' +
              '<div class="project-card-footer-buttons">' +
                '<a class="project-open-btn" href="' + window.location.protocol + '//' + HOSTNAME + ':' + p.port + '/dashboard" target="_blank">Console</a>' +
                '<button class="project-settings-btn" data-slug="' + p.slug + '">Settings</button>' +
                '<button class="project-delete-btn" data-port="' + p.port + '" data-name="' + escapeHtml(p.name) + '">Delete</button>' +
              '</div>' +
              '<span class="project-card-services">:' + p.port + '</span>' +
            '</div>';
          grid.appendChild(card);
        });

        // Bind buttons
        document.querySelectorAll('.project-delete-btn').forEach(function(btn) {
          btn.addEventListener('click', function(e) {
            e.stopPropagation();
            showDeleteModal(this.dataset.name, this.dataset.port);
          });
        });
        document.querySelectorAll('.project-settings-btn').forEach(function(btn) {
          btn.addEventListener('click', function(e) {
            e.stopPropagation();
            showSettingsModal(this.dataset.slug);
          });
        });
      }
      if (retryCount === 0) showToast('Sync complete', 'success');
    } catch (e) {
      if (retryCount < 2) {
        console.warn('Sync failed, retrying...', e);
        setTimeout(() => scanProjects(retryCount + 1), 1000);
      } else {
        grid.innerHTML = emptyState('⚠️', 'Sync Failed', 'Could not fetch project list from admin API.');
        showToast('Failed to load projects: ' + e.message, 'error');
      }
    }
  }

  // ════════════════════════════════════
  // SETTINGS MODAL LOGIC
  // ════════════════════════════════════
  var settingsModal = document.getElementById('settings-modal');
  var currentSettingsSlug = null;
  var currentConfig = null;

  function syncTogglePill(checkboxId, pillId) {
    var cb = document.getElementById(checkboxId);
    var pill = document.getElementById(pillId);
    if (!cb || !pill) return;
    if (cb.checked) pill.classList.add('on'); else pill.classList.remove('on');
  }

  // Wire toggle-switch-wrap clicks to sync pill visual
  document.querySelectorAll('.toggle-switch-wrap').forEach(function(wrap) {
    wrap.addEventListener('click', function(e) {
      var cb = wrap.querySelector('input[type="checkbox"]');
      if (!cb) return;
      cb.checked = !cb.checked;
      var pill = wrap.querySelector('.toggle-pill');
      if (pill) { if (cb.checked) pill.classList.add('on'); else pill.classList.remove('on'); }
      e.preventDefault();
    });
  });


  async function showSettingsModal(slug) {
    currentSettingsSlug = slug;
    settingsModal.classList.add('active');
    
    // Reset fields
    document.getElementById('settings-email-enabled').checked = false;
    document.getElementById('settings-smtp-host').value = '';
    document.getElementById('settings-smtp-port').value = '';
    document.getElementById('settings-smtp-user').value = '';
    document.getElementById('settings-smtp-pass').value = '';
    document.getElementById('settings-smtp-from').value = '';

    try {
      var res = await apiFetch(API + '/admin/projects/' + slug + '/config');
      if (!res.ok) throw new Error('Failed to load config');
      currentConfig = await res.json();

    // Populate SMTP
    if (currentConfig.email) {
      document.getElementById('settings-email-enabled').checked = currentConfig.email.enabled;
      document.getElementById('settings-smtp-host').value = currentConfig.email.host || '';
      document.getElementById('settings-smtp-port').value = currentConfig.email.port || '';
      document.getElementById('settings-smtp-user').value = currentConfig.email.user || '';
      document.getElementById('settings-smtp-pass').value = currentConfig.email.password || '';
      document.getElementById('settings-smtp-from').value = currentConfig.email.from || '';
    }
    // Sync visual toggle pills
    syncTogglePill('settings-email-enabled', 'email-toggle-pill');
    syncTogglePill('settings-hosting-spa', 'spa-toggle-pill');

    // Show warning if SMTP is incomplete
    if (currentConfig.email && currentConfig.email.enabled && !currentConfig.email.host) {
      showToast('Warning: Email is enabled but SMTP host is not configured!', 'info');
    }

    // Populate Database
    if (currentConfig.database) {
      document.getElementById('settings-db-conns').value = currentConfig.database.max_connections || 100;
      document.getElementById('settings-db-cache').value = currentConfig.database.cache_size_mb || 128;
    }

    // Populate Storage
    if (currentConfig.storage) {
      document.getElementById('settings-storage-limit').value = currentConfig.storage.max_file_size || 104857600;
      document.getElementById('settings-storage-types').value = currentConfig.storage.allowed_types || '*/*';
    }

    // Populate Functions
    if (currentConfig.functions) {
      document.getElementById('settings-func-timeout').value = currentConfig.functions.timeout || 60;
      document.getElementById('settings-func-memory').value = currentConfig.functions.memory_limit || 256;
      if (currentConfig.functions.env) {
        var envText = Object.entries(currentConfig.functions.env).map(([k,v]) => k + '=' + v).join('\n');
        document.getElementById('settings-func-env').value = envText;
      }
    }

    // Populate Hosting
    if (currentConfig.hosting) {
      document.getElementById('settings-hosting-spa').checked = currentConfig.hosting.spa_mode !== false;
      if (currentConfig.hosting.headers) {
        var headerText = Object.entries(currentConfig.hosting.headers).map(([k,v]) => k + ': ' + v).join('\n');
        document.getElementById('settings-hosting-headers').value = headerText;
      }
    }

    // Populate Analytics
    if (currentConfig.analytics) {
      document.getElementById('settings-analytics-retention').value = currentConfig.analytics.retention_days || 90;
    }

    // Populate Real-time
    if (currentConfig.realtime) {
      document.getElementById('settings-rt-clients').value = currentConfig.realtime.max_clients || 1000;
    }

    // Populate Services Grid
    var grid = document.getElementById('settings-services-grid');
      grid.innerHTML = '';
      var services = ['auth', 'database', 'storage', 'functions', 'hosting', 'analytics', 'realtime'];
      services.forEach(function(s) {
        var enabled = currentConfig[s] ? currentConfig[s].enabled : false;
        var div = document.createElement('div');
        div.className = 'service-toggle-item';
        div.style.cssText = 'display:flex;align-items:center;gap:0.5rem;background:rgba(255,255,255,0.05);padding:0.5rem;border-radius:6px;border:1px solid var(--border-light)';
        div.innerHTML = 
          '<input type="checkbox" id="set-svc-' + s + '" ' + (enabled ? 'checked' : '') + '>' +
          '<label for="set-svc-' + s + '" style="font-size:0.8rem;text-transform:capitalize;cursor:pointer">' + s + '</label>';
        grid.appendChild(div);
      });

    } catch (e) {
      showToast('Error loading settings: ' + e.message, 'error');
    }
  }

  document.getElementById('save-settings-btn').addEventListener('click', async function() {
    if (!currentSettingsSlug || !currentConfig) return;

    // Update Email Config
    if (!currentConfig.email) currentConfig.email = {};
    currentConfig.email.enabled = document.getElementById('settings-email-enabled').checked;
    currentConfig.email.host = document.getElementById('settings-smtp-host').value;
    currentConfig.email.port = parseInt(document.getElementById('settings-smtp-port').value);
    currentConfig.email.user = document.getElementById('settings-smtp-user').value;
    currentConfig.email.password = document.getElementById('settings-smtp-pass').value;
    currentConfig.email.from = document.getElementById('settings-smtp-from').value;

    // Update Database Config
    if (!currentConfig.database) currentConfig.database = {};
    currentConfig.database.max_connections = parseInt(document.getElementById('settings-db-conns').value);
    currentConfig.database.cache_size_mb = parseInt(document.getElementById('settings-db-cache').value);

    // Update Storage Config
    if (!currentConfig.storage) currentConfig.storage = {};
    currentConfig.storage.max_file_size = parseInt(document.getElementById('settings-storage-limit').value);
    currentConfig.storage.allowed_types = document.getElementById('settings-storage-types').value;

    // Update Functions Config
    if (!currentConfig.functions) currentConfig.functions = {};
    currentConfig.functions.timeout = parseInt(document.getElementById('settings-func-timeout').value);
    currentConfig.functions.memory_limit = parseInt(document.getElementById('settings-func-memory').value);
    
    // Parse Env Vars
    var envText = document.getElementById('settings-func-env').value;
    var envMap = {};
    envText.split('\n').forEach(line => {
      var parts = line.split('=');
      if (parts.length >= 2) envMap[parts[0].trim()] = parts.slice(1).join('=').trim();
    });
    currentConfig.functions.env = envMap;

    // Update Hosting Config
    if (!currentConfig.hosting) currentConfig.hosting = {};
    currentConfig.hosting.spa_mode = document.getElementById('settings-hosting-spa').checked;
    
    // Parse Headers
    var headerText = document.getElementById('settings-hosting-headers').value;
    var headerMap = {};
    headerText.split('\n').forEach(line => {
      var parts = line.split(':');
      if (parts.length >= 2) headerMap[parts[0].trim()] = parts.slice(1).join(':').trim();
    });
    currentConfig.hosting.headers = headerMap;

    // Update Analytics Config
    if (!currentConfig.analytics) currentConfig.analytics = {};
    currentConfig.analytics.retention_days = parseInt(document.getElementById('settings-analytics-retention').value);

    // Update Real-time Config
    if (!currentConfig.realtime) currentConfig.realtime = {};
    currentConfig.realtime.max_clients = parseInt(document.getElementById('settings-rt-clients').value);

    // Update Service Toggles
    var services = ['auth', 'database', 'storage', 'functions', 'hosting', 'analytics', 'realtime'];
    services.forEach(function(s) {
      if (!currentConfig[s]) currentConfig[s] = {};
      currentConfig[s].enabled = document.getElementById('set-svc-' + s).checked;
    });

    try {
      var res = await apiFetch(API + '/admin/projects/' + currentSettingsSlug + '/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(currentConfig)
      });
      if (!res.ok) throw new Error(await res.text());

      showToast('Settings saved! Service is restarting...', 'success');
      settingsModal.classList.remove('active');
      setTimeout(scanProjects, 2000); // Refresh list after restart
    } catch (e) {
      showToast('Failed to save: ' + e.message, 'error');
    }
  });

  document.getElementById('close-settings-modal').addEventListener('click', function() { settingsModal.classList.remove('active'); });
  document.getElementById('close-settings-modal-btn').addEventListener('click', function() { settingsModal.classList.remove('active'); });

  function fetchWithTimeout(url, ms) {
    var controller = new AbortController();
    var timer = setTimeout(function() { controller.abort(); }, ms);
    return fetch(url, { signal: controller.signal }).then(function(res) {
      clearTimeout(timer);
      return res;
    });
  }

  // ════════════════════════════════════
  // DEPLOY & DELETE
  // ════════════════════════════════════

  // DEPLOY
  var deployBtn = document.getElementById('projects-deploy-btn');
  var deployModal = document.getElementById('deploy-modal');
  var closeDeployBtn = document.getElementById('close-deploy-modal');
  var confirmDeployBtn = document.getElementById('confirm-deploy-btn');
  var deployInput = document.getElementById('deploy-project-name');

  if (deployBtn) {
    deployBtn.addEventListener('click', function() {
      deployInput.value = '';
      deployModal.classList.add('active');
      setTimeout(function() { deployInput.focus(); }, 100);
    });
  }
  if (closeDeployBtn) {
    closeDeployBtn.addEventListener('click', function() { deployModal.classList.remove('active'); });
  }

  var selectAllBtn = document.getElementById('deploy-select-all');
  if (selectAllBtn) {
    selectAllBtn.addEventListener('click', function() {
      var checkboxes = deployModal.querySelectorAll('.service-toggle input');
      var allChecked = Array.from(checkboxes).every(function(c) { return c.checked; });
      checkboxes.forEach(function(c) { c.checked = !allChecked; });
      selectAllBtn.textContent = allChecked ? 'Select All' : 'Deselect All';
    });
  }

  if (confirmDeployBtn) {
    confirmDeployBtn.addEventListener('click', async function() {
      var name = deployInput.value.trim();
      if (!name) return showToast('Please enter a project name.', 'error');

      var services = {
        enable_auth: document.getElementById('svc-auth').checked,
        enable_db: document.getElementById('svc-db').checked,
        enable_storage: document.getElementById('svc-storage').checked,
        enable_functions: document.getElementById('svc-functions').checked,
        enable_hosting: document.getElementById('svc-hosting').checked,
        enable_analytics: document.getElementById('svc-analytics').checked,
        enable_realtime: document.getElementById('svc-realtime').checked
      };

      if (!Object.values(services).some(function(v) { return v; })) {
        return showToast('Please select at least one service.', 'error');
      }

      confirmDeployBtn.textContent = 'Deploying...';
      confirmDeployBtn.disabled = true;
      try {
        var res = await apiFetch(API + '/admin/projects', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name: name, ...services })
        });
        if (!res.ok) throw new Error(await res.text());
        showToast('Instance "' + name + '" deployed!', 'success');
        deployModal.classList.remove('active');
        scanProjects();
      } catch (e) {
        showToast('Deployment failed: ' + e.message, 'error');
      } finally {
        confirmDeployBtn.textContent = 'Deploy Instance';
        confirmDeployBtn.disabled = false;
      }
    });
  }

  // DELETE — uses port number for reliable identification
  var currentDeletePort = null;
  var confirmDeleteBtn = document.getElementById('confirm-delete-btn');

  function showDeleteModal(name, port) {
    var modal = document.getElementById('delete-modal');
    if (!modal) return;
    document.getElementById('delete-modal-project-name').textContent = name;
    document.getElementById('delete-modal-project-port').textContent = port;
    currentDeletePort = port;
    modal.classList.add('active');
  }

  if (confirmDeleteBtn) {
    confirmDeleteBtn.addEventListener('click', async function() {
      if (!currentDeletePort) return;
      confirmDeleteBtn.textContent = 'Destroying...';
      confirmDeleteBtn.disabled = true;
      try {
        var res = await apiFetch(API + '/admin/projects/' + encodeURIComponent(currentDeletePort), { method: 'DELETE' });
        var body = await res.text();
        if (!res.ok) throw new Error(body);
        showToast('Instance on port ' + currentDeletePort + ' terminated.', 'success');
        document.getElementById('delete-modal').classList.remove('active');
        currentDeletePort = null;
        scanProjects();
      } catch (e) {
        showToast('Termination failed: ' + e.message, 'error');
      } finally {
        confirmDeleteBtn.textContent = 'Destroy Instance';
        confirmDeleteBtn.disabled = false;
      }
    });
  }

  var closeDelBtn = document.getElementById('close-delete-modal');
  if (closeDelBtn) {
    closeDelBtn.addEventListener('click', function() { document.getElementById('delete-modal').classList.remove('active'); });
  }

  // Close modals on overlay click
  document.querySelectorAll('.dialog-overlay').forEach(function(o) {
    o.addEventListener('click', function(e) {
      if (e.target === o) o.classList.remove('active');
    });
  });

  // ════════════════════════════════════
  // OVERVIEW
  // ════════════════════════════════════
  var activityLog = [];

  async function fetchHealth() {
    try {
      var res = await apiFetch(API + '/health');
      var data = await res.json();

      document.getElementById('server-status').textContent = 'Online';
      document.getElementById('server-status').className = 'status-online';
      document.getElementById('server-version').textContent = 'v' + (data.version || '0.1.0');

      var badge = document.getElementById('topbar-badge');
      if (badge) { badge.textContent = '● Online'; badge.style.cssText = ''; }

      var dot = document.getElementById('pulse-dot');
      var ss = document.getElementById('sidebar-status');
      if (dot) dot.style.background = 'var(--accent-emerald)';
      if (ss) ss.textContent = 'Connected';

      renderServices(data.services);
      logActivity('GET', '/health');

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
      var res = await apiFetch(API + '/analytics/stats');
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
    function authActionBar(isAdminMode) {
      var extra = isAdminMode
        ? '<button id="auth-add-user-btn" class="primary-btn">Add User (Project)</button>' +
          '<button id="auth-delete-user-btn" class="danger-btn">Delete User (Project)</button>'
        : '<button id="auth-add-user-btn" class="primary-btn">Add User</button>';
      return '<div style="display:flex;gap:0.5rem;flex-wrap:wrap;margin-bottom:0.75rem">' + extra + '</div>';
    }

    function bindAuthActions(isAdminMode) {
      var addBtn = document.getElementById('auth-add-user-btn');
      if (addBtn) {
        addBtn.addEventListener('click', async function() {
          var port = '';
          if (isAdminMode) {
            port = window.prompt('Project port for user creation (e.g. 8081):', '8081') || '';
            if (!port) return;
          }
          var email = window.prompt('Email for new user:');
          if (!email) return;
          var password = window.prompt('Password for new user (min 8 chars):');
          if (!password) return;
          var displayName = window.prompt('Display name (optional):') || '';
          var base = isAdminMode ? projectApiBase(port) : API;
          try {
            var res = await apiFetch(base + '/auth/signup', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ email: email.trim(), password: password, display_name: displayName }),
            });
            if (!res.ok) throw new Error(await res.text());
            showToast('User added successfully', 'success');
            fetchAuthData();
          } catch (e) {
            showToast('Add user failed: ' + (e && e.message ? e.message : 'Unknown error'), 'error');
          }
        });
      }

      var delBtn = document.getElementById('auth-delete-user-btn');
      if (delBtn) {
        delBtn.addEventListener('click', async function() {
          var port = window.prompt('Project port for user deletion (e.g. 8081):', '8081') || '';
          if (!port) return;
          var uid = window.prompt('UID to delete:');
          if (!uid) return;
          var ok = window.confirm('Delete this user permanently?');
          if (!ok) return;
          try {
            var base = projectApiBase(port);
            var res = await apiFetch(base + '/auth/admin/users/' + encodeURIComponent(uid.trim()), { method: 'DELETE' });
            if (!res.ok) throw new Error(await res.text());
            showToast('User deleted successfully', 'success');
            fetchAuthData();
          } catch (e) {
            showToast('Delete user failed: ' + (e && e.message ? e.message : 'Unknown error'), 'error');
          }
        });
      }
    }

    if (IS_ADMIN) {
      try {
        var summary = await fetchAdminSummary(false);
        var projects = (summary && summary.projects) || [];
        var totalUsers = 0;
        var signupsToday = 0;
        var activeSessions = 0;
        var html = '<table class="data-table"><thead><tr><th>Project</th><th>Port</th><th>Total Users</th><th>Signups Today</th><th>Active Sessions</th><th>Health</th></tr></thead><tbody>';
        projects.forEach(function(p) {
          var auth = (p.services && p.services.auth) || {};
          if (auth.enabled) {
            totalUsers += Number(auth.users || 0);
            signupsToday += Number(auth.signups_today || 0);
            activeSessions += Number(auth.active_sessions || 0);
          }
          html += '<tr>' +
            '<td>' + escapeHtml(p.slug || p.name || '—') + '</td>' +
            '<td class="mono">' + escapeHtml(String(p.port || '—')) + '</td>' +
            '<td>' + (auth.enabled ? escapeHtml(String(auth.users || 0)) : '<span class="text-muted">Disabled</span>') + '</td>' +
            '<td>' + (auth.enabled ? escapeHtml(String(auth.signups_today || 0)) : '<span class="text-muted">—</span>') + '</td>' +
            '<td>' + (auth.enabled ? escapeHtml(String(auth.active_sessions || 0)) : '<span class="text-muted">—</span>') + '</td>' +
            '<td>' + escapeHtml((p.health || 'unknown').toUpperCase()) + '</td>' +
          '</tr>';
        });
        html += '</tbody></table>';
        document.getElementById('auth-total-users').textContent = String(totalUsers);
        document.getElementById('auth-signups-today').textContent = String(signupsToday);
        document.getElementById('auth-sessions').textContent = String(activeSessions);
        document.getElementById('auth-users-list').innerHTML = authActionBar(true) + (projects.length ? html : emptyState('📦', 'No Projects', 'No deployed projects available in admin registry.'));
        bindAuthActions(true);
        return;
      } catch (e) {
        document.getElementById('auth-users-list').innerHTML = emptyState('⚠️', 'Summary Error', 'Could not fetch project auth summaries.');
        showToast('Failed to load admin auth summary', 'error');
        return;
      }
    }
    logActivity('GET', '/auth/admin/users');
    try {
      var res = await apiFetch(API + '/auth/admin/users');
      if (!res.ok) {
        // Fallback for non-admin users: show current user profile if available.
        var meRes = await apiFetch(API + '/auth/me');
        if (meRes.ok) {
          var meData = await meRes.json();
          var me = meData.user || {};
          document.getElementById('auth-total-users').textContent = '1';
          document.getElementById('auth-signups-today').textContent = '—';
          document.getElementById('auth-sessions').textContent = '—';
          document.getElementById('auth-users-list').innerHTML = authActionBar(false) +
            '<table class="data-table"><thead><tr><th>Email</th><th>UID</th><th>Role</th></tr></thead><tbody>' +
            '<tr><td>' + escapeHtml(me.email || '—') + '</td><td class="mono">' + escapeHtml((me.uid || '—').substring(0, 16)) + '…</td><td>' + (me.admin ? 'Admin' : 'User') + '</td></tr>' +
            '</tbody></table>' +
            '<div style="margin-top:0.75rem;color:var(--text-tertiary);font-size:0.78rem">Showing current signed-in user. Full user list requires admin privileges.</div>';
          bindAuthActions(false);
          return;
        }
        renderAuthSigninPrompt('Session token not found or expired. Sign in to continue.');
        return;
      }
      var data = await res.json();
      var users = data.users || [];
      var sessionsByUID = data.active_sessions_by_uid || {};
      document.getElementById('auth-total-users').textContent = String(data.total ?? users.length);
      document.getElementById('auth-signups-today').textContent = String(data.signups_today ?? '0');
      document.getElementById('auth-sessions').textContent = String(data.active_sessions ?? '0');

      var meUID = '';
      try {
        var meRes = await apiFetch(API + '/auth/me');
        if (meRes.ok) {
          var meData = await meRes.json();
          meUID = (meData.user && meData.user.uid) || '';
        }
      } catch (_) {}

      if (users.length > 0) {
        var html = '<table class="data-table"><thead><tr><th>Email</th><th>UID</th><th>Role</th><th>Sessions</th><th>Created</th><th>Action</th></tr></thead><tbody>';
        users.slice(0, 20).forEach(function(u) {
          var uid = u.uid || '';
          var uidShort = uid ? uid.substring(0, 16) + '…' : '—';
          var isSelf = meUID && uid === meUID;
          var created = (u.created_at && !isNaN(Number(u.created_at))) ? new Date(Number(u.created_at) * 1000).toLocaleString() : '—';
          html += '<tr>' +
            '<td>' + escapeHtml(u.email || '—') + '</td>' +
            '<td class="mono">' + escapeHtml(uidShort) + '</td>' +
            '<td>' + (u.admin ? 'Admin' : 'User') + '</td>' +
            '<td>' + escapeHtml(String(sessionsByUID[uid] ?? 0)) + '</td>' +
            '<td class="text-muted">' + escapeHtml(created) + '</td>' +
            '<td>' +
              (isSelf
                ? '<span class="text-muted">Current</span>'
                : '<button class="project-delete-btn auth-delete-user-btn" data-uid="' + escapeHtml(uid) + '" data-email="' + escapeHtml(u.email || '') + '">Delete</button>') +
            '</td>' +
          '</tr>';
        });
        html += '</tbody></table>';
        document.getElementById('auth-users-list').innerHTML = authActionBar(false) + html;
        bindAuthActions(false);
        document.querySelectorAll('.auth-delete-user-btn').forEach(function(btn) {
          btn.addEventListener('click', async function() {
            var uid = this.getAttribute('data-uid') || '';
            var email = this.getAttribute('data-email') || 'this user';
            if (!uid) return;
            var confirmed = window.confirm('Delete user ' + email + '?\nThis will revoke all active sessions for that user.');
            if (!confirmed) return;
            this.disabled = true;
            this.textContent = 'Deleting...';
            try {
              var delRes = await apiFetch(API + '/auth/admin/users/' + encodeURIComponent(uid), { method: 'DELETE' });
              if (!delRes.ok) {
                var errText = await delRes.text();
                throw new Error(errText || 'Failed to delete user');
              }
              showToast('User deleted successfully', 'success');
              fetchAuthData();
            } catch (err) {
              showToast('Delete failed: ' + (err && err.message ? err.message : 'Unknown error'), 'error');
              this.disabled = false;
              this.textContent = 'Delete';
            }
          });
        });
        showToast('Loaded ' + users.length + ' users', 'success');
      } else {
        document.getElementById('auth-users-list').innerHTML = authActionBar(false) + emptyState('👤', 'No Users Yet', 'Create your first user with the SDK or REST API.', 'curl -X POST ' + API + '/auth/signup \\\n  -H "Content-Type: application/json" \\\n  -d \'{"email":"you@mail.com","password":"secret"}\'');
        bindAuthActions(false);
      }
    } catch(e) {
      renderAuthSigninPrompt('Could not fetch auth data. If the service is online, sign in again to restore session.');
      showToast('Failed to load auth data', 'error');
    }
  }

  // ════════════════════════════════════
  // DATABASE
  // ════════════════════════════════════
  async function fetchDatabaseData() {
    function dbActionBar(isAdminMode) {
      var label = isAdminMode ? '(Project)' : '';
      return (
        '<div style="display:flex;gap:0.5rem;flex-wrap:wrap;margin-bottom:0.75rem">' +
          '<button id="db-add-doc-btn" class="primary-btn">Add Document ' + label + '</button>' +
          '<button id="db-delete-doc-btn" class="danger-btn">Delete Document ' + label + '</button>' +
        '</div>'
      );
    }

    function bindDbActions(isAdminMode) {
      var addBtn = document.getElementById('db-add-doc-btn');
      if (addBtn) {
        addBtn.addEventListener('click', async function() {
          var port = '';
          if (isAdminMode) {
            port = window.prompt('Project port (e.g. 8081):', '8081') || '';
            if (!port) return;
          }
          var collection = window.prompt('Collection name:');
          if (!collection) return;
          var docJson = window.prompt('Document JSON (example: {"name":"item","status":"new"}):', '{"name":"item"}');
          if (!docJson) return;
          try {
            var payload = JSON.parse(docJson);
            var base = isAdminMode ? projectApiBase(port) : API;
            var res = await apiFetch(base + '/db/' + encodeURIComponent(collection.trim()), {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(payload),
            });
            if (!res.ok) throw new Error(await res.text());
            showToast('Document added', 'success');
            fetchDatabaseData();
          } catch (e) {
            showToast('Add document failed: ' + (e && e.message ? e.message : 'Invalid JSON or API error'), 'error');
          }
        });
      }

      var delBtn = document.getElementById('db-delete-doc-btn');
      if (delBtn) {
        delBtn.addEventListener('click', async function() {
          var port = '';
          if (isAdminMode) {
            port = window.prompt('Project port (e.g. 8081):', '8081') || '';
            if (!port) return;
          }
          var collection = window.prompt('Collection name:');
          if (!collection) return;
          var id = window.prompt('Document ID to delete:');
          if (!id) return;
          var ok = window.confirm('Delete this document permanently?');
          if (!ok) return;
          try {
            var base = isAdminMode ? projectApiBase(port) : API;
            var res = await apiFetch(base + '/db/' + encodeURIComponent(collection.trim()) + '/' + encodeURIComponent(id.trim()), {
              method: 'DELETE',
            });
            if (!res.ok) throw new Error(await res.text());
            showToast('Document deleted', 'success');
            fetchDatabaseData();
          } catch (e) {
            showToast('Delete document failed: ' + (e && e.message ? e.message : 'Unknown error'), 'error');
          }
        });
      }
    }

    if (IS_ADMIN) {
      try {
        var summary = await fetchAdminSummary(false);
        var projects = (summary && summary.projects) || [];
        var totalCollections = 0;
        var totalDocuments = 0;
        var html = '<table class="data-table"><thead><tr><th>Project</th><th>Port</th><th>Collections</th><th>Documents</th><th>Health</th></tr></thead><tbody>';
        projects.forEach(function(p) {
          var db = (p.services && p.services.database) || {};
          if (db.enabled) {
            totalCollections += Number(db.collections || 0);
            totalDocuments += Number(db.documents || 0);
          }
          html += '<tr>' +
            '<td>' + escapeHtml(p.slug || p.name || '—') + '</td>' +
            '<td class="mono">' + escapeHtml(String(p.port || '—')) + '</td>' +
            '<td>' + (db.enabled ? escapeHtml(String(db.collections || 0)) : '<span class="text-muted">Disabled</span>') + '</td>' +
            '<td>' + (db.enabled ? escapeHtml(String(db.documents || 0)) : '<span class="text-muted">—</span>') + '</td>' +
            '<td>' + escapeHtml((p.health || 'unknown').toUpperCase()) + '</td>' +
          '</tr>';
        });
        html += '</tbody></table>';
        document.getElementById('db-collections-count').textContent = String(totalCollections);
        document.getElementById('db-documents-count').textContent = String(totalDocuments);
        document.getElementById('db-wal-seq').textContent = 'Cluster';
        document.getElementById('db-collections-list').innerHTML = dbActionBar(true) + (projects.length ? html : emptyState('📦', 'No Projects', 'No deployed projects available in admin registry.'));
        bindDbActions(true);
        return;
      } catch (e) {
        document.getElementById('db-collections-list').innerHTML = emptyState('⚠️', 'Summary Error', 'Could not fetch project database summaries.');
        showToast('Failed to load admin database summary', 'error');
        return;
      }
    }
    logActivity('GET', '/db/collections');
    try {
      var res = await apiFetch(API + '/db/collections');
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
            '<td style="color:var(--accent-indigo);font-size:0.75rem;font-weight:500">Browse →</td></tr>';
        });
        html += '</tbody></table>';
        document.getElementById('db-collections-list').innerHTML = dbActionBar(false) + html;
        bindDbActions(false);

        document.querySelectorAll('.clickable-row[data-collection]').forEach(function(row) {
          row.addEventListener('click', function() { browseCollection(row.dataset.collection); });
        });
      } else {
        document.getElementById('db-collections-list').innerHTML = dbActionBar(false) + emptyState('📂', 'No Collections Yet', 'Create your first collection by saving a document.', 'curl -X POST ' + API + '/db/my_collection \\\n  -H "Content-Type: application/json" \\\n  -d \'{"name":"Hello","status":"World"}\'');
        bindDbActions(false);
      }
    } catch(e) {
      document.getElementById('db-collections-list').innerHTML = emptyState('⚠️', 'Connection Error', 'Could not reach the database service.');
      showToast('Failed to load database', 'error');
    }
  }

  async function browseCollection(name) {
    logActivity('GET', '/db/' + name);
    try {
      var res = await apiFetch(API + '/db/' + encodeURIComponent(name) + '?limit=50');
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
    if (IS_ADMIN) {
      try {
        var summary = await fetchAdminSummary(false);
        var projects = (summary && summary.projects) || [];
        var totalFiles = 0;
        var totalBytes = 0;
        var html = '<table class="data-table"><thead><tr><th>Project</th><th>Port</th><th>Files</th><th>Storage Used</th><th>Health</th></tr></thead><tbody>';
        projects.forEach(function(p) {
          var st = (p.services && p.services.storage) || {};
          if (st.enabled) {
            totalFiles += Number(st.files || 0);
            totalBytes += Number(st.bytes || 0);
          }
          html += '<tr>' +
            '<td>' + escapeHtml(p.slug || p.name || '—') + '</td>' +
            '<td class="mono">' + escapeHtml(String(p.port || '—')) + '</td>' +
            '<td>' + (st.enabled ? escapeHtml(String(st.files || 0)) : '<span class="text-muted">Disabled</span>') + '</td>' +
            '<td>' + (st.enabled ? escapeHtml(formatBytes(Number(st.bytes || 0))) : '<span class="text-muted">—</span>') + '</td>' +
            '<td>' + escapeHtml((p.health || 'unknown').toUpperCase()) + '</td>' +
          '</tr>';
        });
        html += '</tbody></table>';
        document.getElementById('storage-file-count').textContent = String(totalFiles);
        document.getElementById('storage-size').textContent = formatBytes(totalBytes);
        document.getElementById('storage-files-list').innerHTML = projects.length ? html : emptyState('📦', 'No Projects', 'No deployed projects available in admin registry.');
        return;
      } catch (e) {
        document.getElementById('storage-files-list').innerHTML = emptyState('⚠️', 'Summary Error', 'Could not fetch project storage summaries.');
        showToast('Failed to load admin storage summary', 'error');
        return;
      }
    }
    logActivity('GET', '/storage/list');
    try {
      var res = await apiFetch(API + '/storage/list');
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
          html += '<tr><td style="font-weight:500">' + icon + ' ' + escapeHtml(f.name || f.path || '—') + '</td><td class="mono">' + formatBytes(f.size || 0) + '</td><td class="text-muted">' + escapeHtml(f.content_type || '—') + '</td><td><a href="' + API + '/storage/object/' + encodeURIComponent(f.path || f.name || '') + '" target="_blank" style="color:var(--accent-indigo);font-size:0.75rem;font-weight:500;text-decoration:none">Download ↓</a></td></tr>';
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
        var res = await apiFetch(API + '/storage/upload/' + path, {
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
    if (IS_ADMIN) {
      try {
        var summary = await fetchAdminSummary(false);
        var projects = (summary && summary.projects) || [];
        var totalBufferUsed = 0;
        var totalBufferCapacity = 0;
        var html = '<table class="data-table"><thead><tr><th>Project</th><th>Port</th><th>Buffer Used</th><th>Buffer Capacity</th><th>Health</th></tr></thead><tbody>';
        projects.forEach(function(p) {
          var a = (p.services && p.services.analytics) || {};
          if (a.enabled) {
            totalBufferUsed += Number(a.buffer_used || 0);
            totalBufferCapacity += Number(a.buffer_capacity || 0);
          }
          html += '<tr>' +
            '<td>' + escapeHtml(p.slug || p.name || '—') + '</td>' +
            '<td class="mono">' + escapeHtml(String(p.port || '—')) + '</td>' +
            '<td>' + (a.enabled ? escapeHtml(String(a.buffer_used || 0)) : '<span class="text-muted">Disabled</span>') + '</td>' +
            '<td>' + (a.enabled ? escapeHtml(String(a.buffer_capacity || 0)) : '<span class="text-muted">—</span>') + '</td>' +
            '<td>' + escapeHtml((p.health || 'unknown').toUpperCase()) + '</td>' +
          '</tr>';
        });
        html += '</tbody></table>';
        document.getElementById('analytics-events-today').textContent = String(totalBufferUsed);
        document.getElementById('analytics-top-event').textContent = totalBufferCapacity > 0 ? (String(totalBufferUsed) + ' / ' + String(totalBufferCapacity)) : '—';
        document.getElementById('analytics-log-days').textContent = 'Cluster';
        document.getElementById('analytics-counters').innerHTML = projects.length ? html : emptyState('📦', 'No Projects', 'No deployed projects available in admin registry.');
        return;
      } catch (e) {
        document.getElementById('analytics-counters').innerHTML = emptyState('⚠️', 'Summary Error', 'Could not fetch project analytics summaries.');
        showToast('Failed to load admin analytics summary', 'error');
        return;
      }
    }
    logActivity('GET', '/analytics/stats');
    try {
      var res = await apiFetch(API + '/analytics/stats');
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
    if (IS_ADMIN) {
      try {
        var summary = await fetchAdminSummary(false);
        var projects = (summary && summary.projects) || [];
        var c = document.getElementById('settings-info');
        c.innerHTML = '';
        var totals = (summary && summary.totals) || {};
        var items = [
          { label: 'Mode', value: 'Admin Control Center' },
          { label: 'Projects', value: String(totals.projects || projects.length || 0) },
          { label: 'Online', value: String(totals.online || 0) },
          { label: 'Offline', value: String(totals.offline || 0) },
          { label: 'Auth Enabled', value: String(projects.filter(function(p) { return p.services && p.services.auth && p.services.auth.enabled; }).length) },
          { label: 'DB Enabled', value: String(projects.filter(function(p) { return p.services && p.services.database && p.services.database.enabled; }).length) },
          { label: 'Storage Enabled', value: String(projects.filter(function(p) { return p.services && p.services.storage && p.services.storage.enabled; }).length) },
          { label: 'Hosting Enabled', value: String(projects.filter(function(p) { return p.services && p.services.hosting && p.services.hosting.enabled; }).length) },
        ];
        items.forEach(function(item) {
          var div = document.createElement('div');
          div.className = 'settings-item';
          div.innerHTML = '<span class="label">' + escapeHtml(item.label) + '</span><span class="value">' + escapeHtml(item.value) + '</span>';
          c.appendChild(div);
        });

        var danger = document.createElement('div');
        danger.className = 'empty-state';
        danger.style.marginTop = '1rem';
        danger.innerHTML =
          '<div class="empty-icon">🧨</div>' +
          '<h3>Project-Wise Destructive Controls</h3>' +
          '<p>Purges data for a selected project and restarts that project service.</p>' +
          '<div style="display:flex;gap:0.5rem;flex-wrap:wrap;justify-content:center;margin-top:0.75rem">' +
            '<input id="purge-project-slug" class="modal-input" placeholder="project slug (e.g. heimdall)" style="max-width:260px">' +
            '<select id="purge-project-scope" class="modal-input" style="max-width:220px">' +
              '<option value="all">all services</option>' +
              '<option value="auth">auth</option>' +
              '<option value="database">database</option>' +
              '<option value="storage">storage</option>' +
              '<option value="hosting">hosting</option>' +
              '<option value="analytics">analytics</option>' +
              '<option value="functions">functions</option>' +
              '<option value="realtime">realtime</option>' +
            '</select>' +
            '<button id="purge-project-btn" class="danger-btn">Purge Project Data</button>' +
          '</div>';
        c.appendChild(danger);
        var purgeBtn = document.getElementById('purge-project-btn');
        if (purgeBtn) {
          purgeBtn.addEventListener('click', async function() {
            var slugEl = document.getElementById('purge-project-slug');
            var scopeEl = document.getElementById('purge-project-scope');
            var slug = slugEl ? String(slugEl.value || '').trim() : '';
            var scope = scopeEl ? String(scopeEl.value || 'all') : 'all';
            if (!slug) {
              showToast('Enter a project slug first.', 'error');
              return;
            }
            purgeBtn.disabled = true;
            purgeBtn.textContent = 'Purging...';
            try {
              var ok = await requestProjectPurge(API + '/admin/projects/' + encodeURIComponent(slug) + '/purge', [scope]);
              if (ok) {
                showToast('Project purge executed for ' + slug, 'success');
                scanProjects();
              }
            } catch (e) {
              showToast('Purge failed: ' + (e && e.message ? e.message : 'Unknown error'), 'error');
            } finally {
              purgeBtn.disabled = false;
              purgeBtn.textContent = 'Purge Project Data';
            }
          });
        }
        return;
      } catch (e) {
        document.getElementById('settings-info').innerHTML = emptyState('⚠️', 'Summary Error', 'Could not fetch project service matrix.');
        showToast('Failed to load admin settings summary', 'error');
        return;
      }
    }
    logActivity('GET', '/health');
    try {
      var res = await apiFetch(API + '/health');
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

      var childDanger = document.createElement('div');
      childDanger.className = 'empty-state';
      childDanger.style.marginTop = '1rem';
      childDanger.innerHTML =
        '<div class="empty-icon">🧨</div>' +
        '<h3>Instance Destructive Controls</h3>' +
        '<p>Purge selected service data in this child project and restart service.</p>' +
        '<div style="display:flex;gap:0.5rem;flex-wrap:wrap;justify-content:center;margin-top:0.75rem">' +
          '<select id="purge-self-scope" class="modal-input" style="max-width:220px">' +
            '<option value="all">all services</option>' +
            '<option value="auth">auth</option>' +
            '<option value="database">database</option>' +
            '<option value="storage">storage</option>' +
            '<option value="hosting">hosting</option>' +
            '<option value="analytics">analytics</option>' +
            '<option value="functions">functions</option>' +
            '<option value="realtime">realtime</option>' +
          '</select>' +
          '<button id="purge-self-btn" class="danger-btn">Purge This Instance</button>' +
        '</div>';
      c.appendChild(childDanger);
      var purgeSelfBtn = document.getElementById('purge-self-btn');
      if (purgeSelfBtn) {
        purgeSelfBtn.addEventListener('click', async function() {
          var scopeEl = document.getElementById('purge-self-scope');
          var scope = scopeEl ? String(scopeEl.value || 'all') : 'all';
          purgeSelfBtn.disabled = true;
          purgeSelfBtn.textContent = 'Purging...';
          try {
            var ok = await requestProjectPurge(API + '/admin/self/purge', [scope]);
            if (ok) showToast('Instance purge executed', 'success');
          } catch (e) {
            showToast('Purge failed: ' + (e && e.message ? e.message : 'Unknown error'), 'error');
          } finally {
            purgeSelfBtn.disabled = false;
            purgeSelfBtn.textContent = 'Purge This Instance';
          }
        });
      }
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
