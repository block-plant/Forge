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
      var res = await fetch(API + '/admin/projects');
      if (!res.ok) throw new Error(await res.text());
      var projects = await res.json();
      projects = projects || [];

      document.getElementById('projects-active-count').textContent = projects.length;
      document.getElementById('projects-health-status').textContent = projects.length > 0 ? 'Yes ✓' : '—';

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
              '<div class="project-status-dot online"></div>' +
            '</div>' +
            '<div class="project-card-meta">' +
              '<div class="project-meta-row"><span class="meta-label">Port</span><span class="meta-value">' + p.port + '</span></div>' +
              '<div class="project-meta-row"><span class="meta-label">Slug</span><span class="meta-value">' + escapeHtml(p.slug) + '</span></div>' +
              '<div class="project-meta-row"><span class="meta-label">ID</span><span class="meta-value">' + escapeHtml(p.slug) + '</span></div>' +
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
      var res = await fetch(API + '/admin/projects/' + slug + '/config');
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
      var res = await fetch(API + '/admin/projects/' + currentSettingsSlug + '/config', {
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
        var res = await fetch(API + '/admin/projects', {
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
        var res = await fetch(API + '/admin/projects/' + encodeURIComponent(currentDeletePort), { method: 'DELETE' });
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
      var res = await fetch(API + '/health');
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
            '<td style="color:var(--accent-indigo);font-size:0.75rem;font-weight:500">Browse →</td></tr>';
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
