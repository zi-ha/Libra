/* Libra 封面墙前端。零构建，原生 JS，自带样式（不依赖任何 CDN）。
 *
 * 结构分四层，按出现顺序：
 *   1. 工具层：图标、转义、格式化、主题与密度
 *   2. 数据层：接口调用、状态、扫描轮询
 *   3. 视图层：侧栏 / 精选横幅 / 书库宫格 / 封面墙 / 详情 / 管理页
 *   4. 交互层：弹窗、表单、目录选择、路由、初始化
 *
 * 视图层的两条硬规则（前几轮改版踩出来的）：
 *   · 换内容时一次性 innerHTML 赋值，绝不"先清空再逐条插入"；
 *   · 首屏入场动画只播一次（html[data-boot] 闸门），之后的路由切换瞬时呈现。
 *     否则每进一个界面都会整片淡入，看起来就是"闪一下"。
 */
(function () {
  'use strict';

  /* ------------------------------------------------------------------ 图标 */

  var ICONS = {
    book: '<path d="M4 4.5A2.5 2.5 0 0 1 6.5 2H19v16H6.5A2.5 2.5 0 0 0 4 20.5z"/><path d="M19 18v4H6.5A2.5 2.5 0 0 1 4 19.5"/>',
    books: '<rect x="3" y="4" width="4.5" height="17" rx="1"/><rect x="9.75" y="4" width="4.5" height="17" rx="1"/><path d="M17 5.6l3.6 1.3-4.4 15.2-3.4-1.2z"/>',
    library: '<path d="M3 9.5L12 3l9 6.5"/><path d="M5.5 10v10M9.5 10v10M14.5 10v10M18.5 10v10"/><path d="M3 20.5h18"/>',
    home: '<path d="M3.5 11.2L12 4l8.5 7.2"/><path d="M6 9.8V20h12V9.8"/><path d="M10 20v-5.4h4V20"/>',
    star: '<path d="M12 3.2l2.85 5.78 6.38.93-4.62 4.5 1.09 6.36L12 17.77l-5.7 3l1.09-6.36-4.62-4.5 6.38-.93z"/>',
    heart: '<path d="M12 20.7s-7.3-4.5-9-8.7A5.2 5.2 0 0 1 12 6.6a5.2 5.2 0 0 1 9 5.4c-1.7 4.2-9 8.7-9 8.7z"/>',
    globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3c2.4 2.3 3.7 5.4 3.7 9s-1.3 6.7-3.7 9c-2.4-2.3-3.7-5.4-3.7-9S9.6 5.3 12 3z"/>',
    moon: '<path d="M20.4 14.6A8.6 8.6 0 0 1 9.4 3.6a8.6 8.6 0 1 0 11 11z"/>',
    sun: '<circle cx="12" cy="12" r="4.2"/><path d="M12 2v2.4M12 19.6V22M2 12h2.4M19.6 12H22M4.9 4.9l1.7 1.7M17.4 17.4l1.7 1.7M19.1 4.9l-1.7 1.7M6.6 17.4l-1.7 1.7"/>',
    leaf: '<path d="M20.5 3.5s-8.5-1-12.5 3.5-2.5 12-2.5 12 1.5-6 6.5-9.5"/><path d="M11 14c3.5-1 7-1 9.5-.5"/>',
    flame: '<path d="M12 21.5c3.9 0 6.3-2.5 6.3-5.8 0-4.4-4.4-5.9-4.4-9.7 0 0-3.9 1.9-3.9 5.8 0 1.6-1.4 2-1.4 3.9 0 1.4-1 1.9-1 3.8 0 1.4.9 2 1.4 2z"/>',
    cube: '<path d="M12 2.7l8.6 4.9v9.8L12 22.3 3.4 17.4V7.6z"/><path d="M12 12.1l8.6-4.5M12 12.1v10.2M12 12.1L3.4 7.6"/>',
    music: '<path d="M9 18.2V6.4l10-2v11.8"/><circle cx="6.8" cy="18.2" r="2.2"/><circle cx="16.8" cy="16.2" r="2.2"/>',
    film: '<rect x="3" y="4.5" width="18" height="15" rx="2.2"/><path d="M8.2 4.5v15M15.8 4.5v15M3 12h18"/>',
    folder: '<path d="M3 7.2A2.2 2.2 0 0 1 5.2 5h3.6l2.2 2.6h8A2.2 2.2 0 0 1 21 9.8v7.4A2.2 2.2 0 0 1 18.8 19.4H5.2A2.2 2.2 0 0 1 3 17.2z"/>',
    compass: '<circle cx="12" cy="12" r="9"/><path d="M15.6 8.4l-2.1 5.1-5.1 2.1 2.1-5.1z"/>',
    sparkle: '<path d="M11 3.2l1.7 4.4 4.4 1.7-4.4 1.7L11 15.4 9.3 11 4.9 9.3 9.3 7.6z"/><path d="M18 14.4l.8 2 2 .8-2 .8-.8 2-.8-2-2-.8 2-.8z"/>',
    shield: '<path d="M12 3l7.2 2.7v5.6c0 4.3-2.9 8.1-7.2 9.4-4.3-1.3-7.2-5.1-7.2-9.4V5.7z"/>',
    cloud: '<path d="M7 18.8h10.2a3.9 3.9 0 0 0 .4-7.8A5.5 5.5 0 0 0 7.2 9.5 4.2 4.2 0 0 0 7 18.8z"/>',
    code: '<path d="M9 8l-4.5 4L9 16"/><path d="M15 8l4.5 4L15 16"/>',
    download: '<path d="M12 3.5v11"/><path d="M7.2 10.2L12 15l4.8-4.8"/><path d="M4.5 19.5h15"/>',
    refresh: '<path d="M20 11A8 8 0 0 0 6.3 6.3L4 8.5"/><path d="M4 4v4.5h4.5"/><path d="M4 13a8 8 0 0 0 13.7 4.7L20 15.5"/><path d="M20 20v-4.5h-4.5"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    pencil: '<path d="M4.5 19.5h3.6L18.6 9a2.1 2.1 0 0 0-3-3L5.1 16.5z"/><path d="M14.4 7.2l2.4 2.4"/>',
    trash: '<path d="M4.5 6.5h15"/><path d="M9 6.5V4.8A1.3 1.3 0 0 1 10.3 3.5h3.4A1.3 1.3 0 0 1 15 4.8v1.7"/><path d="M6.5 6.5l.9 12.2A1.8 1.8 0 0 0 9.2 20.4h5.6a1.8 1.8 0 0 0 1.8-1.7l.9-12.2"/>',
    back: '<path d="M15 6l-6 6 6 6"/>',
    chevron: '<path d="M9 6l6 6-6 6"/>',
    up: '<path d="M12 19V5"/><path d="M5 12l7-7 7 7"/>',
    image: '<rect x="3" y="4.5" width="18" height="15" rx="2.2"/><circle cx="8.6" cy="10" r="1.8"/><path d="M4 17l4.7-4.4 3.6 3.3 3-2.7L20 16.6"/>',
    folderOpen: '<path d="M3 7.2A2.2 2.2 0 0 1 5.2 5h3.6l2.2 2.6h8A2.2 2.2 0 0 1 21 9.8v.7"/><path d="M3 9.5h18.4L19 18.6a1.8 1.8 0 0 1-1.8 1.4H5.2A2.2 2.2 0 0 1 3 17.8z"/>',
    check: '<circle cx="12" cy="12" r="9"/><path d="M8.4 12.3l2.5 2.5 4.7-5"/>',
    alert: '<circle cx="12" cy="12" r="9"/><path d="M12 7.5v5.2M12 16.2v.2"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11.4v5M12 8.1v.2"/>',
    gridS: '<rect x="4" y="4" width="4.4" height="4.4" rx="1.1"/><rect x="9.8" y="4" width="4.4" height="4.4" rx="1.1"/><rect x="15.6" y="4" width="4.4" height="4.4" rx="1.1"/><rect x="4" y="9.8" width="4.4" height="4.4" rx="1.1"/><rect x="9.8" y="9.8" width="4.4" height="4.4" rx="1.1"/><rect x="15.6" y="9.8" width="4.4" height="4.4" rx="1.1"/><rect x="4" y="15.6" width="4.4" height="4.4" rx="1.1"/><rect x="9.8" y="15.6" width="4.4" height="4.4" rx="1.1"/><rect x="15.6" y="15.6" width="4.4" height="4.4" rx="1.1"/>',
    gridM: '<rect x="4" y="4" width="7" height="7" rx="1.6"/><rect x="13" y="4" width="7" height="7" rx="1.6"/><rect x="4" y="13" width="7" height="7" rx="1.6"/><rect x="13" y="13" width="7" height="7" rx="1.6"/>',
    gridL: '<rect x="4" y="4" width="16" height="16" rx="2.4"/>',
    user: '<circle cx="12" cy="8.4" r="3.7"/><path d="M4.6 20.2a7.6 7.6 0 0 1 14.8 0"/>',
    key: '<circle cx="8" cy="12" r="3.6"/><path d="M11.6 12H20l-.1 3M17.4 12v3"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="M20 20l-3.6-3.6"/>',
    play: '<path d="M8 5.5l11 6.5-11 6.5z"/>',
    dice: '<rect x="4" y="4" width="16" height="16" rx="3.2"/><circle cx="9" cy="9" r="1.3"/><circle cx="15" cy="15" r="1.3"/><circle cx="12" cy="12" r="1.3"/><circle cx="15" cy="9" r="1.3"/><circle cx="9" cy="15" r="1.3"/>',
  };

  var DEFAULT_ICON = 'builtin:book';

  /* 当前登录者的角色，由服务端写在 <html data-role> 上。
     前端据此隐藏越权的入口 —— 真正的权限拦截在后端 adminOnly 中间件，
     这里只是不让用户点到一个必然 403 的按钮。 */
  var IS_ADMIN = document.documentElement.getAttribute('data-role') === 'admin';

  var BOOT_ATTR = 'data-boot';
  var bootGateOpen = true;

  /* 首屏入场动画播完就摘掉闸门。之后所有路由切换都是瞬时呈现，
     不再重放 .rise 淡入 —— 那正是"每进一个界面闪一下"的来源。 */
  function closeBootGate() {
    if (!bootGateOpen) return;
    bootGateOpen = false;
    setTimeout(function () {
      document.documentElement.removeAttribute(BOOT_ATTR);
    }, 400);
  }

  /* ------------------------------------------------------------------ 工具 */

  function $(sel, root) { return (root || document).querySelector(sel); }
  function $$(sel, root) { return Array.prototype.slice.call((root || document).querySelectorAll(sel)); }

  function esc(s) {
    return String(s === null || s === undefined ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function fmtBytes(n) {
    n = Number(n) || 0;
    if (n <= 0) return '0 B';
    var units = ['B', 'KB', 'MB', 'GB', 'TB'];
    var i = 0;
    while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
    return (i === 0 ? n : n.toFixed(2)) + ' ' + units[i];
  }

  function fmtTime(v) {
    if (!v) return '—';
    var d = new Date(v);
    if (isNaN(d.getTime()) || d.getFullYear() < 1970) return '—';
    function p(x) { return x < 10 ? '0' + x : '' + x; }
    return d.getFullYear() + '-' + p(d.getMonth() + 1) + '-' + p(d.getDate()) +
      ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
  }

  /* 相对时间：横幅里"3 天前入库"比一串时间戳好读得多 */
  function fmtAgo(v) {
    if (!v) return '';
    var d = new Date(v);
    if (isNaN(d.getTime()) || d.getFullYear() < 1970) return '';
    var s = Math.max(0, (Date.now() - d.getTime()) / 1000);
    if (s < 90) return '刚刚';
    if (s < 3600) return Math.round(s / 60) + ' 分钟前';
    if (s < 86400) return Math.round(s / 3600) + ' 小时前';
    if (s < 86400 * 30) return Math.round(s / 86400) + ' 天前';
    if (s < 86400 * 365) return Math.round(s / 86400 / 30) + ' 个月前';
    return Math.round(s / 86400 / 365) + ' 年前';
  }

  function debounce(fn, wait) {
    var t = null;
    return function () {
      var args = arguments, self = this;
      clearTimeout(t);
      t = setTimeout(function () { fn.apply(self, args); }, wait);
    };
  }

  function iconHTML(icon, cls) {
    icon = icon || DEFAULT_ICON;
    if (icon.indexOf('upload:') === 0) {
      var name = icon.slice('upload:'.length);
      return '<img src="/icons/' + encodeURIComponent(name) + '" alt=""' +
        (cls ? ' class="' + cls + '"' : '') + '>';
    }
    var key = icon.indexOf('builtin:') === 0 ? icon.slice('builtin:'.length) : 'book';
    var path = ICONS[key] || ICONS.book;
    var solid = key.indexOf('grid') === 0;
    return '<svg viewBox="0 0 24 24"' + (cls ? ' class="' + cls + '"' : '') +
      (solid ? ' fill="currentColor"' : ' fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"') +
      '>' + path + '</svg>';
  }

  function svg(path, cls) {
    return '<svg' + (cls ? ' class="' + cls + '"' : '') +
      ' viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"' +
      ' stroke-linecap="round" stroke-linejoin="round">' + path + '</svg>';
  }

  /* 把任意字符串映射成一个稳定的色相（0~359）。
     书库图标块、无封面占位的底色都从这里取色：同一座库、同一本书
     每次刷新颜色都一样，但整体看上去又是五颜六色的。 */
  function hashHue(s) {
    s = String(s || '');
    var h = 0;
    for (var i = 0; i < s.length; i++) {
      h = (h * 31 + s.charCodeAt(i)) | 0;
    }
    return Math.abs(h) % 360;
  }

  /* 写进内联 style 的令牌：--tint 色相、--tint-s 饱和度、--tint-l 亮度。
     深色主题下亮度稍高、浅色下稍低，图标块在两套主题里都够清楚。 */
  function tintStyle(seed) {
    var light = currentTheme() === 'light';
    return '--tint:' + hashHue(seed) + ';--tint-s:64%;--tint-l:' + (light ? 48 : 52) + '%';
  }

  /* ------------------------------------------------------------------ 主题与密度 */

  var THEME_KEY = 'libra-theme';
  var DENSITY_KEY = 'libra-density';

  function currentTheme() {
    return document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
  }

  function paintThemeButton() {
    var btn = $('#theme-btn');
    if (!btn) return;
    var light = currentTheme() === 'light';
    btn.innerHTML = svg(light ? ICONS.moon : ICONS.sun);
    btn.title = light ? '切换到深色' : '切换到浅色';
    btn.setAttribute('aria-label', btn.title);
    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute('content', light ? '#ffffff' : '#15171d');
  }

  /* 画布底色由 <head> 里的内联脚本预先写上，用来消除样式表到达前的白闪。
     内联样式不会跟着 data-theme 走，所以主题一变就要同步刷新，
     否则滚动溢出区会残留旧主题的底色。取值直接读 CSS 令牌，避免两处硬编码。 */
  function paintCanvas() {
    var root = document.documentElement;
    var theme = currentTheme();
    root.style.colorScheme = theme;
    var bg = '';
    try { bg = getComputedStyle(root).getPropertyValue('--bg').trim(); } catch (e) { /* 忽略 */ }
    root.style.backgroundColor = bg || (theme === 'light' ? '#f4f5f7' : '#0c0d10');
  }

  function toggleTheme() {
    var next = currentTheme() === 'light' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', next);
    paintCanvas();
    try { localStorage.setItem(THEME_KEY, next); } catch (e) { /* 忽略 */ }
    paintThemeButton();
    // 图标块的亮度是按主题给的，换主题后强制重刷一遍侧栏
    renderSidebar(true);
  }

  function currentDensity() {
    var d = document.documentElement.getAttribute('data-density');
    return (d === 's' || d === 'l') ? d : 'm';
  }

  function applyDensity(d) {
    if (d === 'm') document.documentElement.removeAttribute('data-density');
    else document.documentElement.setAttribute('data-density', d);
    try { localStorage.setItem(DENSITY_KEY, d); } catch (e) { /* 忽略 */ }
    paintDensitySeg();
  }

  function paintDensitySeg() {
    var seg = $('#density-seg');
    if (!seg) return;
    var cur = currentDensity();
    $$('button', seg).forEach(function (b) {
      b.setAttribute('aria-pressed', String(b.getAttribute('data-density') === cur));
    });
  }

  /* ------------------------------------------------------------------ API */

  function api(path, options) {
    var opts = options || {};
    var init = { method: opts.method || 'GET', credentials: 'same-origin' };
    if (opts.body !== undefined) {
      init.headers = { 'Content-Type': 'application/json' };
      init.body = JSON.stringify(opts.body);
    }
    return fetch(path, init).then(function (res) {
      if (res.status === 401) { location.href = '/login'; throw new Error('登录已过期'); }
      return res.json().catch(function () { return { ok: false, error: '服务端响应异常 (' + res.status + ')' }; })
        .then(function (body) {
          if (!res.ok || !body || body.ok !== true) {
            throw new Error((body && body.error) || ('请求失败 (' + res.status + ')'));
          }
          return body.data;
        });
    });
  }

  function apiUpload(path, formData) {
    return fetch(path, { method: 'POST', body: formData, credentials: 'same-origin' })
      .then(function (res) {
        return res.json().catch(function () { return { ok: false, error: '服务端响应异常 (' + res.status + ')' }; })
          .then(function (body) {
            if (!res.ok || !body || body.ok !== true) {
              throw new Error((body && body.error) || '上传失败');
            }
            return body.data;
          });
      });
  }

  /* ------------------------------------------------------------------ Toast */

  var TOAST_ICON = { ok: ICONS.check, err: ICONS.alert, info: ICONS.info };

  function toast(msg, type) {
    type = type || 'info';
    var node = document.createElement('div');
    node.className = 'toast ' + type;
    node.innerHTML = svg(TOAST_ICON[type] || ICONS.info) + '<span>' + esc(msg) + '</span>';
    $('#toasts').appendChild(node);
    setTimeout(function () {
      node.classList.add('out');
      setTimeout(function () { node.remove(); }, 340);
    }, 3800);
  }

  /* ------------------------------------------------------------------ 状态 */

  var state = {
    libraries: [],
    libraryId: 0,       // 0 = 首页 / -1 = 全部书籍 / >0 = 某个书库
    q: '',
    format: '',
    sort: 'title:asc',
    page: 1,
    pageSize: 60,
    total: 0,
    pages: 0,
    books: [],
    loading: false,
    scanning: [],
    pollTimer: null,
    iconCache: null,
    detailId: 0,
    wallSig: '',        // 上次封面墙查询条件，用于返回时直接复用已有卡片
    featured: null,     // 首页横幅本轮内容（每会话取一次，扫描 / 数据源变更后失效）
    featuredIdx: 0,
    featuredForce: false, // 下一次取横幅时跳过服务端一小时缓存（管理员点了"换一批"）
    heroTimer: null,
    libCovers: {},      // 书库宫格拼贴：libraryId -> [封面 URL]
    lastElapsed: 0,
    sentinel: null
  };

  function libraryById(id) {
    for (var i = 0; i < state.libraries.length; i++) {
      if (state.libraries[i].id === id) return state.libraries[i];
    }
    return null;
  }

  /* ------------------------------------------------------------------ 侧栏 */

  function renderSidebar(force) {
    var nav = $('#sidebar');
    if (!nav) return;

    var total = state.libraries.reduce(function (a, l) { return a + (l.book_count || 0); }, 0);
    var routeName = parseHash().name;
    var onHome = routeName === 'home';
    var onAll = routeName === 'wall' && state.libraryId === -1;
    var onAdmin = routeName === 'admin' || routeName === 'libraries' || routeName === 'users' ||
      routeName === 'featured';

    // 内容没变就不重建 DOM —— 每次路由都重写 innerHTML 会让侧栏跟着闪一下。
    var sig = state.libraryId + '|' + routeName + '|' + total + '|' + currentTheme() + '|' +
      state.libraries.map(function (l) {
        return l.id + ':' + l.name + ':' + l.icon + ':' + (l.book_count || 0) +
          ':' + (l.scanning ? 1 : 0) + ':' + (l.last_scan_state || '');
      }).join(',');
    if (!force && nav.getAttribute('data-sig') === sig) return;
    nav.setAttribute('data-sig', sig);

    var html =
      '<a href="#/" class="side-item' + (onHome ? ' active' : '') + '">' +
        '<span class="side-icon neutral">' + iconHTML('builtin:home') + '</span>' +
        '<span class="side-name">首页</span>' +
      '</a>' +
      '<a href="#/all" class="side-item' + (onAll ? ' active' : '') + '">' +
        '<span class="side-icon neutral">' + iconHTML('builtin:books') + '</span>' +
        '<span class="side-name">全部书籍</span>' +
        '<span class="side-count">' + total + '</span>' +
      '</a>' +
      '<div class="side-fold">书库</div>';

    if (!state.libraries.length) {
      html += '<div style="padding:8px 10px;font-size:12px;color:var(--text-3)">还没有书库</div>';
    }

    state.libraries.forEach(function (lib) {
      var active = routeName === 'wall' && state.libraryId === lib.id;
      html += '<a href="#/lib/' + lib.id + '" class="side-item' + (active ? ' active' : '') + '"' +
        ' title="' + esc(lib.name) + ' · ' + esc(lib.path) + '">' +
        '<span class="side-icon" style="' + tintStyle(lib.name) + '">' + iconHTML(lib.icon) + '</span>' +
        '<span class="side-name">' + esc(lib.name) + '</span>' +
        (lib.scanning
          ? svg('<path d="M12 3a9 9 0 1 0 9 9"/>', 'side-spin')
          : '<span class="side-count">' + (lib.book_count || 0) + '</span>') +
        '</a>';
    });

    html += '<div class="side-sep"></div>';
    if (IS_ADMIN) {
      html += '<a href="#/admin" class="side-item' + (onAdmin ? ' active' : '') + '">' +
        '<span class="side-icon neutral">' + iconHTML('builtin:shield') + '</span>' +
        '<span class="side-name">管理后台</span>' +
        '</a>';
    } else {
      html += '<a href="#/libraries" class="side-item' + (routeName === 'libraries' ? ' active' : '') + '">' +
        '<span class="side-icon neutral">' + iconHTML('builtin:folder') + '</span>' +
        '<span class="side-name">书库列表</span>' +
        '</a>';
    }

    nav.innerHTML = html;
  }

  function renderSidebarStats() {
    var node = $('#sidebar-stats');
    if (!node) return;

    var sig = state.libraries.map(function (l) {
      return l.id + ':' + (l.book_count || 0) + ':' + (l.epub_count || 0) + ':' + (l.pdf_count || 0);
    }).join(',');
    if (node.getAttribute('data-sig') === sig) return;
    node.setAttribute('data-sig', sig);

    if (!state.libraries.length) {
      node.innerHTML = '<div class="k">藏书</div><div class="v" style="font-size:14px;font-weight:600;color:var(--text-2)">还没有书库</div>';
      return;
    }

    var epub = 0, pdf = 0;
    state.libraries.forEach(function (l) {
      epub += l.epub_count || 0;
      pdf += l.pdf_count || 0;
    });

    node.innerHTML =
      '<div class="k">藏书总数</div>' +
      '<div class="v">' + (epub + pdf) + '<small>本</small></div>' +
      '<div class="split">' +
        '<span>EPUB <b>' + epub + '</b></span>' +
        '<span>PDF <b>' + pdf + '</b></span>' +
        '<span>书库 <b>' + state.libraries.length + '</b></span>' +
      '</div>';
  }

  function loadLibraries() {
    return api('/api/libraries').then(function (data) {
      state.libraries = data.libraries || [];
      var scanning = state.libraries.filter(function (l) { return l.scanning; }).map(function (l) { return l.id; });
      if (scanning.length) {
        scanning.forEach(function (id) {
          if (state.scanning.indexOf(id) === -1) state.scanning.push(id);
        });
        startPolling();
      }
      renderSidebar();
      renderSidebarStats();
      return state.libraries;
    });
  }

  /* ------------------------------------------------------------------ 扫描进度 */

  function startPolling() {
    if (state.pollTimer) return;
    state.pollTimer = setInterval(tickPoll, 1200);
    tickPoll();
  }

  function stopPolling() {
    if (state.pollTimer) { clearInterval(state.pollTimer); state.pollTimer = null; }
    $('#scan-bar').classList.add('u-hidden');
  }

  function renderScanBar(agg) {
    var bar = $('#scan-bar');
    if (!state.scanning.length) { bar.classList.add('u-hidden'); return; }
    bar.classList.remove('u-hidden');
    var pct = agg.total > 0 ? Math.round((agg.processed / agg.total) * 100) : 4;
    var names = state.scanning.map(function (id) {
      var lib = libraryById(id);
      return lib ? lib.name : ('#' + id);
    }).join('、');
    $('#scan-text').textContent = '正在扫描 ' + names +
      (agg.total > 0 ? '（' + agg.processed + ' / ' + agg.total + '）' : '（统计目录中…）');
    $('#scan-progress').style.width = pct + '%';
  }

  function tickPoll() {
    if (!state.scanning.length) { stopPolling(); return; }
    var ids = state.scanning.slice();
    Promise.all(ids.map(function (id) {
      return api('/api/libraries/' + id + '/scan')
        .then(function (d) { return d.status; })
        .catch(function () { return null; });
    })).then(function (list) {
      var still = [], finished = [], agg = { total: 0, processed: 0 };

      list.forEach(function (st) {
        if (!st) return;
        if (st.state === 'scanning') {
          still.push(st.library_id);
          agg.total += st.total || 0;
          agg.processed += st.processed || 0;
        } else {
          finished.push(st);
        }
      });

      state.scanning = still;
      renderScanBar(agg);

      if (finished.length) {
        finished.forEach(function (st) {
          var lib = libraryById(st.library_id);
          var name = lib ? lib.name : ('#' + st.library_id);
          if (st.state === 'error') {
            toast('「' + name + '」扫描失败：' + (st.message || '未知错误'), 'err');
          } else {
            var parts = ['新增 ' + (st.added || 0), '更新 ' + (st.updated || 0), '跳过 ' + (st.skipped || 0)];
            if (st.removed) parts.push('移除 ' + st.removed);
            if (st.failed) parts.push('失败 ' + st.failed);
            toast('「' + name + '」扫描完成：' + parts.join('，') +
              (st.elapsed_sec ? '，耗时 ' + st.elapsed_sec.toFixed(1) + 's' : ''), 'ok');
          }
        });

        // 扫完以后新的封面可能已经写盘，清掉精选与拼贴缓存，重新渲染时再取。
        state.featured = null;
        state.libCovers = {};
        loadLibraries().then(function () {
          var r = parseHash();
          if (r.name === 'libraries') renderLibrariesView();
          else if (r.name === 'wall') loadBooks(true);
          else if (r.name === 'home') renderHome(true);
        });
      }

      if (!state.scanning.length) stopPolling();
    });
  }

  function triggerScan(libraryId) {
    return api('/api/libraries/' + libraryId + '/scan', { method: 'POST' }).then(function () {
      if (state.scanning.indexOf(libraryId) === -1) state.scanning.push(libraryId);
      var lib = libraryById(libraryId);
      if (lib) lib.scanning = true;
      renderSidebar(true);
      startPolling();
      toast('已开始扫描「' + (lib ? lib.name : libraryId) + '」', 'info');
    });
  }

  /* ------------------------------------------------------------------ 顶栏标题 */

  function setTopbar(title, sub) {
    var t = $('#tb-title');
    var s = $('#tb-sub');
    if (t) t.textContent = title || 'Libra';
    // 没有副标题时不渲染那个"·"，否则真会出现只有一个小圆点的空行
    var bar = $('#topbar');
    if (bar) bar.classList.toggle('no-sub', !sub);
    renderCrumbSep(sub);
    if (s) s.textContent = sub || '';
    document.title = title && title !== 'Libra' ? title + ' · Libra' : 'Libra';
  }

  /* 顶栏右侧的排序 / 深浅色 / 标记墙只属于"全部书籍"页。
     首页、书籍详情、管理页把这一块整体撤掉：标题就是书名，排序对详情页没有意义，
     留着反而让顶栏显得很吵。没有这些工具时面包屑自动吃掉剩余宽度、居中显示。 */
  function setTopbarTools(show) {
    var bar = $('#topbar');
    if (bar) bar.classList.toggle('page-topbar', !!show);
  }

  /* 面包屑之间那个"·"必须有副标题才有意义。
     不能只靠 CSS 隐藏：现在 setTopbar() 会先同步把 no-sub 类挂上，
     但路由顺序一变就可能出现"只有书名 + 一个孤零零的点"的中间帧。
     所以直接把分隔符的可见性交给 JS 决定。 */
  function renderCrumbSep(sub) {
    var sep = $('.crumb-sep');
    if (sep) sep.style.display = sub ? '' : 'none';
  }

  /* ------------------------------------------------------------------ 封面 */

  function coverURL(b) { return b && b.cover_url ? b.cover_url : ''; }
  function thumbURL(b) { return b && b.thumb_url ? b.thumb_url : coverURL(b); }

  /* 封面底座：有图就放 img，没有就放带书名与主题色的占位块。
     默认使用 /thumbs/ 缩略图（400px），详情页传 { original: true } 用原图。 */
  function coverHTML(b, title, opts) {
    opts = opts || {};
    title = title || b.title || '';
    var url = opts.original ? coverURL(b) : thumbURL(b);
    var img = url
      ? '<img src="' + esc(url) + '" alt=""' + (opts.eager ? '' : ' loading="lazy"') +
        ' decoding="async" onerror="this.remove()">'
      : '';
    return '<div class="fallback" role="img" aria-label="' + esc(title) + '" style="' + tintStyle(title) + '">' +
      '<span>' + esc(title) + '</span></div>' + img;
  }

  /* ------------------------------------------------------------------ 精选横幅 */

  function loadFeatured() {
    if (state.featured) return Promise.resolve(state.featured);
    // force=1：绕过服务端一小时的随机结果缓存，立刻重新抽一轮。
    var url = '/api/featured?limit=10' + (state.featuredForce ? '&refresh=1' : '');
    state.featuredForce = false;
    return api(url).then(function (d) {
      var list = (d.books || []).filter(function (b) { return !!coverURL(b); });
      if (!list.length) list = d.books || [];
      state.featured = list;
      return list;
    }).catch(function () {
      state.featured = [];
      return [];
    });
  }

  /* 首页横幅右上角不留任何入口：换一批、数据源配置都在"管理后台 → 横幅数据源"里。
     横幅对每个人来说都只是横幅，管理员也一样。 */

  /* 清掉缓存、重新抽一轮并重绘横幅（只在管理后台保存之后调用）。
     服务端那侧的缓存也一并跳过（refresh=1），所见即所得。 */
  function reshuffleHero() {
    var wrap = $('#hero-wrap');
    if (!wrap) return;
    stopHeroRotation();
    state.featured = null;
    state.featuredForce = true;
    state.featuredIdx = 0;
    loadFeatured().then(function (list) {
      if ($('#view').getAttribute('data-view') !== 'home') return;
      if (!document.body.contains(wrap)) return;
      preloadHeroImages(list);
      wrap.innerHTML = heroHTML(list);
      bindHero();
    });
  }

  function heroSub(b) {
    return b.author || '未知作者';
  }

  function heroMetaHTML(b) {
    var bits = [];
    bits.push('<span><b>' + esc(String(b.format || '').toUpperCase()) + '</b></span>');
    bits.push('<span class="dot"></span><span>' + esc(b.size_human || fmtBytes(b.size)) + '</span>');
    if (b.library_name) {
      bits.push('<span class="dot"></span><span>' + esc(b.library_name) + '</span>');
    }
    var ago = fmtAgo(b.created_at);
    if (ago) {
      bits.push('<span class="dot"></span><span>' + esc(ago) + '入库</span>');
    }
    return bits.join('');
  }

  /* 书名越长字号越小。按"视觉宽度"分档而不是按字符数：
     中日韩字符的宽度约等于一个 em，拉丁字母/数字大约是一半，
     直接数字符会把英文长书名误判成短书名。 */
  function titleSizeClass(title) {
    var s = String(title || '');
    var w = 0;
    for (var i = 0; i < s.length; i++) {
      var c = s.charCodeAt(i);
      w += c > 0x2e80 ? 1 : (c === 0x20 ? 0.35 : 0.55);
    }
    if (w > 34) return ' is-xlong';
    if (w > 20) return ' is-long';
    return '';
  }

  /* 横幅：左边是"当前这本"的封面缩略图，右边是书名 / 作者 / 信息 / 按钮。
     大图仍由整块横幅的背景承担 —— 同样是这一本的封面。
     翻页靠两侧按钮、底部指示点与自动轮播。

     防闪的关键：整块内容（封面、书名、作者、元信息、按钮）都装在一个 .hero-slide 里，
     所有可轮播的书各自预加载一份背景层，再提前解码好下一张封面。
     换页时只做两件事：旧幻灯片淡出、新幻灯片淡入，背景层交叉溶解 ——
     不存在"文字先清空再填"或"图片还没下载完就置换背景"的中间帧，
     所以不会出现一闪一闪。 */
  function heroHTML(list) {
    if (!list.length) {
      // 横幅数据源里一本书都没有（比如只开了每日推荐、书单还是空的）。
      // 首页不放任何设置入口：管理员去「管理后台 → 横幅数据源」处理，
      // 普通成员干脆看不到这块。
      if (!IS_ADMIN) return '';
      return '<section class="hero hero-empty rise" id="hero" aria-label="精选">' +
          '<div class="hero-empty-inner">' +
            '<span class="hero-empty-icon">' + svg(ICONS.dice) + '</span>' +
            '<h2>首页横幅还没有内容</h2>' +
            '<p>把几本书加进「每日推荐」，或者选一个书库作为横幅的数据源。</p>' +
            '<a href="#/admin" class="btn btn-primary">' + svg(ICONS.shield) + '去管理后台设置</a>' +
          '</div>' +
        '</section>';
    }
    var cur = list[state.featuredIdx % list.length];
    var bg = coverURL(cur);
    var thumbSrc = thumbURL(cur);
    var n = list.length;

    var dots = '';
    for (var i = 0; i < n; i++) {
      var on = i === state.featuredIdx % n;
      dots += '<button type="button" class="hero-dot' + (on ? ' on' : '') +
        '" data-idx="' + i + '" data-count="' + (i + 1) + ' / ' + n +
        '" aria-label="第 ' + (i + 1) + ' 本" aria-current="' + on + '"></button>';
    }

    var nav = n > 1
      ? '<button type="button" class="hero-nav prev" id="hero-prev" aria-label="上一本精选">' +
          svg(ICONS.back) + '</button>' +
        '<button type="button" class="hero-nav next" id="hero-next" aria-label="下一本精选">' +
          svg(ICONS.chevron) + '</button>'
      : '';

    return '<section class="hero rise" id="hero" aria-label="精选">' +
        // 背景层：每本书一层，全部提前加载好，换页只切 .is-on
        '<div class="hero-bgs" id="hero-bgs" aria-hidden="true">' +
          list.map(function (b, i) {
            var u = coverURL(b);
            return '<div class="hero-bg' + (i === state.featuredIdx % n ? ' is-on' : '') + '"' +
              ' data-idx="' + i + '" data-url="' + esc(u) + '"' +
              (u ? ' style="background-image:url(&quot;' + esc(u) + '&quot;)"' : '') + '></div>';
          }).join('') +
        '</div>' +
        '<div class="hero-shade"></div>' +
        '<div class="hero-slides">' +
          list.map(function (b, i) {
            return heroSlideHTML(b, i === state.featuredIdx % n);
          }).join('') +
        '</div>' +
        nav +
        (n > 1 ? '<div class="hero-dots" id="hero-dots">' + dots + '</div>' : '') +
      '</section>';
  }

  /* 一张幻灯片 = 一张封面 + 一份文案。非当前幻灯片也一起渲染出来（只是不可见），
     这样翻页时没有一次 DOM 写入发生在"看得见"的时刻。 */
  function heroSlideHTML(b, on) {
    var hasCover = !!coverURL(b);
    return '<div class="hero-slide' + (on ? ' is-on' : '') + '" data-idx="' + b.id + '">' +
        '<div class="hero-inner">' +
          /* 左侧封面缩略图：固定 2:3、object-fit: cover 铺满，
             封面本身不是 2:3 时居中裁剪，绝不出现黑边白边 */
          '<a class="hero-thumb' + (hasCover ? '' : ' is-empty') + '" href="#/book/' + b.id + '"' +
            ' tabindex="' + (on ? '0' : '-1') + '" aria-label="' + esc(b.title) + '">' +
            '<img alt="" decoding="async"' +
              (thumbURL(b) ? ' src="' + esc(thumbURL(b)) + '"' : '') + '></a>' +
          '<div class="hero-copy">' +
            '<h2 class="hero-title' + titleSizeClass(b.title) + '">' +
              '<span>' + esc(b.title) + '</span></h2>' +
            '<div class="hero-sub">' + esc(heroSub(b)) + '</div>' +
            '<div class="hero-meta">' + heroMetaHTML(b) + '</div>' +
            '<div class="hero-actions">' +
              '<a class="btn btn-primary" href="#/book/' + b.id + '" tabindex="' + (on ? '0' : '-1') + '">' +
                svg(ICONS.play) + '查看详情</a>' +
              '<a class="btn" href="/download/' + b.id + '" tabindex="' + (on ? '0' : '-1') + '">' +
                svg(ICONS.download) + '下载文件</a>' +
            '</div>' +
          '</div>' +
        '</div>' +
      '</div>';
  }

  /* 预加载：把 list 里所有背景图与缩略图都拉进浏览器缓存。
     之前换页时是把新 URL 直接写到 background-image 上，图片没下载完的那一瞬间
     背景是空的 —— 这就是"一闪一闪"的主因之一。 */
  function preloadHeroImages(list) {
    (list || []).forEach(function (b) {
      [coverURL(b), thumbURL(b)].forEach(function (u) {
        if (!u) return;
        var im = new Image();
        im.decoding = 'async';
        im.src = u;
        if (im.decode) im.decode().catch(function () {});
      });
    });
  }

  /* 换一张精选：只切 class，不动任何文字与 URL。
     旧幻灯片保持可见直到新幻灯片完全淡入（.is-out 的 transition-delay 与淡入同长），
     两层短暂叠在一起 —— 叠着切换永远不会有中间的空帧。 */
  function selectFeatured(idx) {
    var list = state.featured || [];
    if (!list.length) return;
    idx = (idx + list.length) % list.length;
    if (idx === state.featuredIdx && $('#hero .hero-slide.is-on')) return;
    state.featuredIdx = idx;

    $$('#hero .hero-slide').forEach(function (n, i) {
      var on = i === idx;
      n.classList.toggle('is-on', on);
      n.classList.toggle('is-out', !on);
      n.setAttribute('aria-hidden', String(!on));
      // 隐藏的幻灯片不参与 Tab 焦点链，否则键盘会跳到看不见的按钮上
      $$('a', n).forEach(function (a) { a.setAttribute('tabindex', on ? '0' : '-1'); });
    });

    $$('#hero-bgs .hero-bg').forEach(function (n, i) {
      n.classList.toggle('is-on', i === idx);
    });

    $$('#hero-dots .hero-dot').forEach(function (n, i) {
      var on = i === idx;
      n.classList.toggle('on', on);
      // 窄屏只显示一个"3 / 10"的小计数，这里把序号同步过去
      n.setAttribute('data-count', (i + 1) + ' / ' + list.length);
      n.setAttribute('aria-current', String(on));
    });
  }

  function startHeroRotation() {
    stopHeroRotation();
    var list = state.featured || [];
    if (list.length < 2) return;
    state.heroTimer = setInterval(function () {
      // 页签不可见时不转，省电
      if (document.hidden) return;
      selectFeatured(state.featuredIdx + 1);
    }, 7000);
  }

  function stopHeroRotation() {
    if (state.heroTimer) { clearInterval(state.heroTimer); state.heroTimer = null; }
  }

  function bindHero() {
    var hero = $('#hero');
    if (!hero) return;

    // 左右按钮：手动翻一张后重新开始计时，避免刚点完就被自动轮播抢走
    function stepHero(dir) {
      stopHeroRotation();
      selectFeatured(state.featuredIdx + dir);
      startHeroRotation();
    }
    var prev = $('#hero-prev', hero);
    var next = $('#hero-next', hero);
    if (prev) prev.addEventListener('click', function () { stepHero(-1); });
    if (next) next.addEventListener('click', function () { stepHero(1); });

    // 点指示点直接跳到那一张，并重新计时
    $$('#hero-dots .hero-dot', hero).forEach(function (n) {
      n.addEventListener('click', function () {
        stopHeroRotation();
        selectFeatured(Number(this.getAttribute('data-idx')));
        startHeroRotation();
      });
    });

    // 鼠标停在横幅上时不自动翻页：正在读简介却被换走，体验很差
    hero.addEventListener('mouseenter', stopHeroRotation);
    hero.addEventListener('mouseleave', startHeroRotation);

    startHeroRotation();
  }

  /* ------------------------------------------------------------------ 横幅数据源（管理后台）
     首页横幅的内容从哪来，由「管理后台 → 横幅数据源」决定：
       · 每日推荐 —— 一份人工维护的书单，随时增删
       · 随机书库 —— 从选定书库里随机取（不选就是全部书库）
     两者可以同时开，开启后每次轮播都从合并结果里随机抽。
     首页横幅本身不放任何设置入口，也不标注来源。 */

  /* 页面里用的局部状态：书单顺序就是提交顺序。 */
  var FEAT_CTX = null;

  function renderFeaturedSourceView() {
    var view = $('#view');
    stopHeroRotation();
    view.setAttribute('data-view', 'featured');

    view.innerHTML =
      '<a href="#/admin" class="back-link">' + svg(ICONS.back) + '返回管理后台</a>' +
      '<div class="page-head">' +
        '<div class="grow">' +
          '<h1 class="page-title">横幅数据源</h1>' +
          '<div class="page-sub">决定首页轮播展示哪些书；多个来源同时开启时，会从合并后的结果里随机抽取</div>' +
        '</div>' +
        '<div class="page-tools">' +
          '<button type="button" class="btn" id="feat-shuffle">' + svg(ICONS.refresh) + '换一批看看</button>' +
        '</div>' +
      '</div>' +
      '<div class="feat-layout">' +
        '<div class="feat-main" id="feat-body">' +
          '<div class="sk-line" style="width:40%"></div>' +
          '<div class="sk-line" style="width:72%"></div>' +
          '<div class="sk-line" style="width:56%"></div>' +
        '</div>' +
        // 右侧预览：和首页横幅长得一样，改完立刻能看到效果
        '<aside class="feat-side">' +
          '<div class="feat-side-head"><h3>首页预览</h3>' +
            '<span class="feat-side-sub" id="feat-preview-sub"></span></div>' +
          '<div class="feat-preview" id="feat-preview"></div>' +
          '<p class="feat-side-note">' + svg(ICONS.info) +
            '<span>轮播内容每小时随机一次，同一小时内顺序保持不变；保存设置后会立刻换一批。</span></p>' +
        '</aside>' +
      '</div>';

    setTopbar('横幅数据源', '管理后台');
    setTopbarTools(false);
    closeBootGate();

    FEAT_CTX = {
      q: '',
      saving: false,
      items: [],
      cfg: { picks_enabled: true, library_ids: [0] },
      showError: function (msg) {
        var box = $('#feat-msg');
        if (!box) { toast(msg, 'err'); return; }
        box.innerHTML = svg(ICONS.alert) + '<span>' + esc(msg) + '</span>';
        box.classList.remove('u-hidden');
      }
    };
    bindFeaturedConfig(FEAT_CTX);

    var shuffle = $('#feat-shuffle');
    if (shuffle) {
      shuffle.addEventListener('click', function () {
        // 跳过服务端缓存重抽一轮，并红一下按钮表示"换过了"
        state.featured = null;
        state.featuredForce = true;
        loadFeaturedPreview(true);
        shuffle.classList.add('is-pressed');
        setTimeout(function () { shuffle.classList.remove('is-pressed'); }, 400);
      });
    }

    loadFeaturedConfig(FEAT_CTX, function (err) {
      var body = $('#feat-body');
      if (err && body) {
        body.innerHTML = '<div class="alert error">' + svg(ICONS.alert) +
          '<span>读取失败：' + esc(err.message) + '</span></div>';
      }
    });

    // 右侧预览和设置同时开始拉：横幅内容与书单是两路请求，互不等待
    loadFeaturedPreview(true);
  }

  /* 右侧预览：直接复用首页横幅的渲染函数，但只读 —— 不带翻页按钮、不自动轮播。
     点预览里的书仍然能进详情页，和首页一致。 */
  function loadFeaturedPreview(force) {
    var box = $('#feat-preview');
    if (!box) return;
    if (!force && state.featured) { paintFeaturedPreview(); return; }
    if (force) state.featured = null;
    loadFeatured().then(function (list) {
      if (!$('#feat-preview')) return;
      state.featurePreviewList = list;
      paintFeaturedPreview();
    });
  }

  function paintFeaturedPreview() {
    var box = $('#feat-preview');
    if (!box) return;
    var list = state.featurePreviewList || [];
    var sub = $('#feat-preview-sub');
    if (sub) sub.textContent = list.length ? ('本轮 ' + list.length + ' 本') : '本轮为空';
    if (!list.length) {
      box.innerHTML = '<div class="feat-preview-empty">' + svg(ICONS.dice) +
        '<span>按当前设置，首页横幅不会有内容</span></div>';
      return;
    }
    // 预览只显示当前这一本，静态摆放；首页那边才是会动的轮播。
    // 图层顺序必须和首页一致：背景 → 遮罩 → 幻灯片 → 指示点，否则幻灯片会被遮罩盖住。
    box.innerHTML = '<div class="hero is-preview">' +
        '<div class="hero-bgs" aria-hidden="true"></div>' +
        '<div class="hero-shade"></div>' +
        '<div class="hero-slides">' +
          heroSlideHTML(list[0], true) +
        '</div>' +
        '<div class="hero-dots feat-preview-dots">' + list.map(function () {
          return '<span class="hero-dot"></span>';
        }).join('') + '</div>' +
      '</div>';
    var bgs = box.querySelector('.hero-bgs');
    if (bgs && list[0] && coverURL(list[0])) {
      bgs.innerHTML = '<div class="hero-bg is-on" style="background-image:url(&quot;' +
        esc(coverURL(list[0])) + '&quot;)"></div>';
    }
    // 预览里只有当前这一本，链接和按钮都是活的
    $$('.hero-slide .hero-thumb, .hero-slide .hero-actions a', box).forEach(function (a) {
      a.setAttribute('tabindex', '0');
    });
  }

  function loadFeaturedConfig(ctx, done) {
    Promise.all([
      api('/api/daily-picks'),
      api('/api/featured/source')
    ]).then(function (res) {
      ctx.items = res[0].books || [];
      ctx.cfg = {
        picks_enabled: res[0].picks_enabled !== false,
        library_ids: (res[0].library_ids || [0])
      };
      renderFeaturedConfig(ctx);
      done && done(null);
    }).catch(function (err) {
      done && done(err);
    });
  }

  function renderFeaturedConfig(ctx) {
    var body = $('#feat-body');
    if (!body) return;

    var libs = state.libraries;
    var selected = ctx.cfg.library_ids || [0];
    var allSelected = selected.indexOf(0) !== -1 || !selected.length;

    var libChips = '<button type="button" class="feat-lib' + (allSelected ? ' on' : '') +
        '" data-lib="0" aria-pressed="' + allSelected + '">全部书库</button>' +
      libs.map(function (l) {
        var on = !allSelected && selected.indexOf(l.id) !== -1;
        return '<button type="button" class="feat-lib' + (on ? ' on' : '') +
          '" data-lib="' + l.id + '" aria-pressed="' + on + '">' +
          '<span class="feat-lib-dot" style="' + tintStyle(l.name) + '"></span>' +
          esc(l.name) +
          '<span class="feat-lib-count">' + (l.book_count || 0) + '</span></button>';
      }).join('');

    var list = '';
    if (!ctx.items.length) {
      list = '<div class="feat-empty">' +
          '<span class="feat-empty-icon">' + svg(ICONS.dice) + '</span>' +
          '<p>书单还是空的。在下面搜索书名或作者，把想推荐的书加进来。</p>' +
        '</div>';
    } else {
      list = '<ul class="feat-list" id="feat-list">' + ctx.items.map(function (b, i) {
        return '<li class="feat-item" data-id="' + b.id + '" data-idx="' + i + '" draggable="true">' +
            '<span class="feat-grip" title="拖动排序">' + iconHTML('builtin:grids') + '</span>' +
            '<span class="feat-pick-cover">' + coverHTML(b, b.title) + '</span>' +
            '<span class="feat-pick-body">' +
              '<span class="feat-pick-title u-clamp1">' + esc(b.title) + '</span>' +
              '<span class="feat-pick-meta">' + esc(b.author || '未知作者') +
                (b.library_name ? ' · ' + esc(b.library_name) : '') + '</span>' +
            '</span>' +
            '<button type="button" class="btn btn-sm btn-danger feat-remove" data-remove="' + b.id + '" ' +
              'title="移出每日推荐">' + svg(ICONS.trash) + '</button>' +
          '</li>';
      }).join('') + '</ul>';
    }

    body.innerHTML =
      '<section class="feat-block">' +
        '<div class="feat-block-head">' +
          '<div>' +
            '<h4>每日推荐</h4>' +
            '<p>人工挑选的书，轮播时会优先出现。当前 ' + ctx.items.length + ' 本。</p>' +
          '</div>' +
          '<label class="switch" title="每日推荐是否参与轮播">' +
            '<input id="feat-picks-on" type="checkbox"' + (ctx.cfg.picks_enabled ? ' checked' : '') +
              ' aria-label="每日推荐是否参与轮播">' +
            '<span class="switch-track"></span>' +
          '</label>' +
        '</div>' +
        list +
        '<div class="feat-search">' +
          svg(ICONS.search) +
          '<input id="feat-q" type="search" class="input" placeholder="搜索书名、作者、文件名，加入每日推荐…" ' +
            'autocomplete="off" value="' + esc(ctx.q) + '">' +
        '</div>' +
        '<div class="feat-results" id="feat-results"></div>' +
      '</section>' +

      '<section class="feat-block">' +
        '<div class="feat-block-head">' +
          '<div>' +
            '<h4>随机书库</h4>' +
            '<p>开启后，从选中的书库里随机取书参与轮播；不选就是全部书库。</p>' +
          '</div>' +
        '</div>' +
        '<div class="feat-libs" id="feat-libs">' + libChips + '</div>' +
      '</section>' +

      '<div class="feat-note">' + svg(ICONS.info) +
        '<span>保存后首页横幅立刻换一批。内容本身每小时随机一次，同一小时内顺序保持不变。</span></div>' +
      '<div id="feat-msg" class="alert error u-hidden"></div>';

    bindFeaturedBody(ctx);
    if (ctx.q) searchPickCandidates(ctx);
  }

  function bindFeaturedConfig(ctx) {
    // 保存类操作：统一走这里，失败回显
    ctx.save = function (promise, okMsg) {
      if (ctx.saving) return Promise.resolve();
      ctx.saving = true;
      return promise.then(function () {
        ctx.saving = false;
        if (okMsg) toast(okMsg, 'ok');
        return loadFeaturedConfig(ctx);
      }).catch(function (err) {
        ctx.saving = false;
        ctx.showError(err.message);
      });
    };
    ctx.showError = function (msg) {
      var box = $('#feat-msg');
      if (!box) { toast(msg, 'err'); return; }
      box.innerHTML = svg(ICONS.alert) + '<span>' + esc(msg) + '</span>';
      box.classList.remove('u-hidden');
    };
  }

  /* 绑定整页的交互（开关 / 书库胶囊 / 移出 / 拖动排序 / 搜索）。
     每次 renderFeaturedConfig() 重绘之后都会重新调一次。 */
  function bindFeaturedBody(ctx) {
    // 每日推荐开关
    var toggle = $('#feat-picks-on');
    if (toggle) {
      toggle.addEventListener('change', function () {
        var on = this.checked;
        this.disabled = true;
        ctx.cfg.picks_enabled = on;
        ctx.save(api('/api/featured/source', {
          method: 'PUT',
          body: { picks_enabled: on }
        }), on ? '每日推荐已参与轮播' : '每日推荐已暂停').then(function () {
          var t = $('#feat-picks-on');
          if (t) t.disabled = false;
        });
      });
    }

    // 书库多选
    $$('#feat-libs .feat-lib').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var id = Number(this.getAttribute('data-lib'));
        var cur = ctx.cfg.library_ids || [0];
        var next;
        if (id === 0) {
          next = [0]; // "全部书库"是排他的
        } else {
          var rest = cur.filter(function (x) { return x > 0 && x !== id; });
          if (cur.indexOf(id) !== -1) {
            next = rest; // 取消勾选
          } else {
            next = rest.concat([id]);
          }
          // 全选等于"全部书库"
          if (next.length && next.length === state.libraries.length) next = [0];
        }
        if (!next.length) next = [0];
        ctx.cfg.library_ids = next;
        renderFeaturedConfig(ctx);
        ctx.save(api('/api/featured/source', {
          method: 'PUT',
          body: { library_ids: next }
        }));
      });
    });

    // 移出书单
    $$('#feat-list [data-remove]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var id = Number(this.getAttribute('data-remove'));
        var row = this.closest('.feat-item');
        btn.disabled = true;
        if (row) row.classList.add('is-removing');
        ctx.save(api('/api/daily-picks/' + id, { method: 'DELETE' }), '已移出每日推荐');
      });
    });

    // 拖拽排序：拖动结束后按 DOM 顺序把 ID 列表整体提交
    var list = $('#feat-list');
    if (list) {
      var dragId = null;
      list.addEventListener('dragstart', function (ev) {
        var li = ev.target.closest('.feat-item');
        if (!li) return;
        dragId = li.getAttribute('data-id');
        li.classList.add('is-dragging');
        if (ev.dataTransfer) {
          ev.dataTransfer.effectAllowed = 'move';
          // Firefox 不设 data 就不会开始拖动
          try { ev.dataTransfer.setData('text/plain', dragId); } catch (e) {}
        }
      });
      list.addEventListener('dragend', function () {
        dragId = null;
        $$('#feat-list .feat-item').forEach(function (n) {
          n.classList.remove('is-dragging', 'is-drop-target');
        });
      });
      list.addEventListener('dragover', function (ev) {
        if (!dragId) return;
        ev.preventDefault();
        var over = ev.target.closest('.feat-item');
        $$('#feat-list .feat-item').forEach(function (n) { n.classList.remove('is-drop-target'); });
        if (over && over.getAttribute('data-id') !== dragId) over.classList.add('is-drop-target');
      });
      list.addEventListener('drop', function (ev) {
        if (!dragId) return;
        ev.preventDefault();
        var over = ev.target.closest('.feat-item');
        var dragged = $('#feat-list .feat-item[data-id="' + dragId + '"]');
        if (!dragged) return;
        if (over && over !== dragged) {
          var r = over.getBoundingClientRect();
          var after = ev.clientY > r.top + r.height / 2;
          list.insertBefore(dragged, after ? over.nextSibling : over);
        }
        var ids = $$('#feat-list .feat-item').map(function (n) { return Number(n.getAttribute('data-id')); });
        ctx.save(api('/api/daily-picks', { method: 'PUT', body: { book_ids: ids } }));
      });
    }

    // 搜索候选
    var q = $('#feat-q');
    if (q) {
      q.addEventListener('input', debounce(function () {
        ctx.q = this.value.trim();
        if (!ctx.q) {
          var box = $('#feat-results');
          if (box) box.innerHTML = '';
          return;
        }
        searchPickCandidates(ctx);
      }, 320));
    }
  }

  /* 搜索可加入书单的书。已经在书单里的显示"已加入"，点击无效果。 */
  function searchPickCandidates(ctx) {
    var box = $('#feat-results');
    if (!box) return;
    box.innerHTML = '<div class="feat-hint">搜索中…</div>';
    var q = encodeURIComponent(ctx.q || '');
    api('/api/daily-picks/candidates?q=' + q).then(function (d) {
      if (ctx.q === '' || !$('#feat-results')) return;
      var books = (d.books || []).slice(0, 12);
      if (!books.length) {
        box.innerHTML = '<div class="feat-hint">没有找到匹配的书</div>';
        return;
      }
      box.innerHTML = books.map(function (b) {
        var added = !!b.in_picks;
        return '<div class="feat-cand" data-cand="' + b.id + '">' +
            '<span class="feat-cand-cover">' + coverHTML(b, b.title) + '</span>' +
            '<span class="feat-cand-title u-clamp2">' + esc(b.title) + '</span>' +
            '<span class="feat-cand-meta">' + esc(b.author || '未知作者') + '</span>' +
            '<button type="button" class="btn btn-sm' + (added ? '' : ' btn-primary') + '" ' +
              (added ? 'disabled' : '') + '>' +
              (added ? '已加入' : svg(ICONS.plus) + '加入') + '</button>' +
          '</div>';
      }).join('');

      $$('#feat-results [data-cand]').forEach(function (row) {
        var btn = row.querySelector('button');
        btn.addEventListener('click', function () {
          var id = Number(row.getAttribute('data-cand'));
          btn.disabled = true;
          ctx.save(api('/api/daily-picks', {
            method: 'POST',
            body: { book_id: id }
          }), '已加入每日推荐').then(function () {
            // 重新搜索一次，让这一行变成"已加入"
            searchPickCandidates(ctx);
          });
        });
      });
    }).catch(function (err) {
      box.innerHTML = '<div class="alert error">' + svg(ICONS.alert) +
        '<span>' + esc(err.message) + '</span></div>';
    });
  }

  /* ------------------------------------------------------------------ 首页
     首页只做"入口"：顶部一条精选横幅，下面每个书库一行。
     具体的浏览、筛选、翻页，都在点进库里之后。
     像 Emby 的首页那样，一行只放十来个，看清是哪座库、最近进了什么就够了。 */
  function renderHome(force) {
    var view = $('#view');
    var already = view.getAttribute('data-view') === 'home' && $('#hero-wrap');

    if (already && !force) {
      setTopbar('首页', state.libraries.length + ' 个书库');
      setTopbarTools(false);
      return;
    }

    stopHeroRotation();
    setTopbarTools(false);
    view.setAttribute('data-view', 'home');
    view.innerHTML =
      '<div id="hero-wrap"></div>' +
      (state.libraries.length
        ? homeLibrariesHTML() +
          '<div id="home-rows">' +
            state.libraries.map(homeRowHTML).join('') +
          '</div>'
        : '<div class="empty" id="home-empty"><div class="empty-mark">' +
            iconHTML('builtin:library') + '</div><h2>还没有书库</h2>' +
            '<p>到「管理书库」里把一个装满电子书的目录挂进来。</p>' +
            (IS_ADMIN
              ? '<a href="#/libraries" class="btn btn-primary">' + svg(ICONS.plus) + '新建书库</a>'
              : '') +
          '</div>');

    setTopbar('首页', state.libraries.length + ' 个书库');

    // 横幅数据回来再插入，避免整页布局先跳一下
    loadFeatured().then(function (list) {
      if (view.getAttribute('data-view') !== 'home') return;
      var wrap = $('#hero-wrap');
      if (!wrap) return;
      preloadHeroImages(list);
      wrap.innerHTML = heroHTML(list);
      bindHero();
    });

    // 每座库拉一行：先铺骨架，数据到了整行替换
    state.libraries.slice(0, 6).forEach(function (lib) {
      bindRail(lib.id);
      loadHomeRow(lib);
    });

    // 书库入口卡的封面：每座库取三张，异步回来后再替换图标占位
    loadLibraryCovers();
    bindHomeLibs();
  }

  /* 首页的书库入口：像 Emby 的库卡片那样，直接用该库最近几本书的封面
     拼成一张横向卡片，库名压在最下面。不显示分区标题、数量或入口链接。 */
  function homeLibraryTileHTML(lib) {
    var label =
      '<span class="home-lib-label">' +
        '<span class="home-lib-badge" style="' + tintStyle(lib.name) + '">' + iconHTML(lib.icon) + '</span>' +
        '<span class="home-lib-name">' + esc(lib.name) + '</span>' +
      '</span>';

    // 管理员上传过封面就直接用；没有才走自动拼贴
    if (lib.cover_url) {
      var customCover = lib.cover_thumb_url || lib.cover_url;
      return '<a class="home-lib" href="#/lib/' + lib.id + '" title="' + esc(lib.name) + '">' +
          '<span class="home-lib-single"><img src="' + esc(customCover) + '" alt=""' +
            ' loading="lazy" decoding="async" onerror="this.remove()"></span>' +
          '<span class="home-lib-veil"></span>' +
          label +
        '</a>';
    }

    var covers = state.libCovers[lib.id] || [];
    if (!covers.length) {
      // 还没取到封面时先放图标占位，数据回来后整排重绘
      return '<a class="home-lib is-empty" href="#/lib/' + lib.id + '" title="' + esc(lib.name) + '">' +
          '<span class="home-lib-fallback" style="' + tintStyle(lib.name) + '">' + iconHTML(lib.icon) + '</span>' +
          '<span class="home-lib-veil"></span>' +
          label +
        '</a>';
    }

    // 三张竖封面并排，正好把横向卡片填满；不足三张时循环补齐
    var cells = '';
    for (var i = 0; i < 3; i++) {
      var url = covers[i % covers.length];
      cells += '<span class="home-lib-cell"><img src="' + esc(url) + '" alt="" loading="lazy"' +
        ' decoding="async" onerror="this.remove()"></span>';
    }

    return '<a class="home-lib" href="#/lib/' + lib.id + '" title="' + esc(lib.name) + '">' +
        '<span class="home-lib-collage">' + cells + '</span>' +
        '<span class="home-lib-veil"></span>' +
        label +
      '</a>';
  }

  function homeLibrariesHTML() {
    return '<section class="home-libs" aria-label="书库">' +
        '<div class="home-lib-strip" id="home-lib-strip">' +
          '<button class="rail-btn home-lib-nav prev" type="button" aria-label="向左滚动" disabled>' +
            svg(ICONS.back) + '</button>' +
          '<button class="rail-btn home-lib-nav next" type="button" aria-label="向右滚动" disabled>' +
            svg(ICONS.chevron) + '</button>' +
          '<div class="home-lib-row" id="home-lib-row">' +
            state.libraries.map(homeLibraryTileHTML).join('') +
          '</div>' +
        '</div>' +
      '</section>';
  }

  function updateHomeLibNav() {
    var strip = $('#home-lib-strip');
    var row = $('#home-lib-row');
    if (!strip || !row) return;
    var prev = $('.home-lib-nav.prev', strip);
    var next = $('.home-lib-nav.next', strip);
    if (!prev || !next) return;

    var max = row.scrollWidth - row.clientWidth;
    var noOverflow = max <= 2;
    strip.classList.toggle('no-overflow', noOverflow);
    prev.disabled = noOverflow || row.scrollLeft <= 2;
    next.disabled = noOverflow || row.scrollLeft >= max - 2;
  }

  function scrollHomeLibs(dir) {
    var row = $('#home-lib-row');
    if (!row) return;
    var step = Math.max(Math.round(row.clientWidth * 0.8), 240);
    var max = Math.max(0, row.scrollWidth - row.clientWidth);
    var left = Math.min(Math.max(row.scrollLeft + dir * step, 0), max);
    if (row.scrollTo) row.scrollTo({ left: left, behavior: 'smooth' });
    else row.scrollLeft = left;
  }

  function bindHomeLibs() {
    var strip = $('#home-lib-strip');
    var row = $('#home-lib-row');
    if (!strip || !row) return;

    if (strip.getAttribute('data-nav-bound') !== '1') {
      strip.setAttribute('data-nav-bound', '1');
      var prev = $('.home-lib-nav.prev', strip);
      var next = $('.home-lib-nav.next', strip);
      if (prev) prev.addEventListener('click', function () { scrollHomeLibs(-1); });
      if (next) next.addEventListener('click', function () { scrollHomeLibs(1); });
      row.addEventListener('scroll', updateHomeLibNav, { passive: true });
    }
    updateHomeLibNav();
  }

  /* 书库卡片封面：每座库取最近几本书的封面。结果缓存在 state.libCovers，
     同一会话内来回切换首页不重复请求。 */
  function loadLibraryCovers() {
    var pending = state.libraries.filter(function (lib) {
      return !lib.cover_url && !state.libCovers[lib.id];
    });
    if (!pending.length) return Promise.resolve();

    return Promise.all(pending.map(function (lib) {
      return api('/api/books?library_id=' + lib.id + '&page_size=8&sort=created&order=desc')
        .then(function (d) {
          state.libCovers[lib.id] = (d.books || []).map(thumbURL).filter(Boolean).slice(0, 3);
        })
        .catch(function () { state.libCovers[lib.id] = []; });
    })).then(function () {
      var view = $('#view');
      var row = $('#home-lib-row');
      if (!view || !row || view.getAttribute('data-view') !== 'home') return;
      var left = row.scrollLeft;
      row.innerHTML = state.libraries.map(homeLibraryTileHTML).join('');
      row.scrollLeft = left;
      bindHomeLibs();
    });
  }

  /* 一行 = 库名（可点进该库）+ 右侧"查看全部" + 一排海报。
     海报行不显示滚动条，改用两侧的圆形按钮翻页（Emby 式）。 */
  function homeRowHTML(lib) {
    return '<section class="rail-row" id="rail-' + lib.id + '">' +
        '<div class="section-head">' +
          '<a class="rail-title" href="#/lib/' + lib.id + '">' +
            '<span class="rail-badge" style="' + tintStyle(lib.name) + '">' + iconHTML(lib.icon) + '</span>' +
            '<h2>' + esc(lib.name) + '</h2>' +
            '<span class="count">' + (lib.book_count || 0) + '</span>' +
          '</a>' +
          '<span class="grow"></span>' +
          '<a class="more" href="#/lib/' + lib.id + '">查看全部' + svg(ICONS.chevron) + '</a>' +
        '</div>' +
        '<div class="rail" id="rail-body-' + lib.id + '">' +
          '<button class="rail-btn prev" type="button" aria-label="向左滚动" disabled>' +
            svg(ICONS.back) + '</button>' +
          '<button class="rail-btn next" type="button" aria-label="向右滚动" disabled>' +
            svg(ICONS.chevron) + '</button>' +
          '<div class="rail-track" id="rail-track-' + lib.id + '">' + railSkeletonHTML() + '</div>' +
        '</div>' +
      '</section>';
  }

  function railSkeletonHTML(n) {
    var out = '<div class="rail-scroller">';
    for (var i = 0; i < (n || 8); i++) out += '<div class="rail-item"><div class="sk-cover"></div></div>';
    return out + '</div>';
  }

  function loadHomeRow(lib) {
    api('/api/books?library_id=' + lib.id + '&page_size=12&sort=created&order=desc')
      .then(function (d) {
        var track = $('#rail-track-' + lib.id);
        if (!track) return; // 已经离开首页
        var books = d.books || [];
        if (!books.length) {
          track.innerHTML = '<div class="rail-empty">这座书库还是空的</div>';
          bindRail(lib.id);
          return;
        }
        track.innerHTML = '<div class="rail-scroller">' +
          books.map(function (b) { return railItemHTML(b); }).join('') +
          '</div>';
        bindRail(lib.id);
      })
      .catch(function () {
        var track = $('#rail-track-' + lib.id);
        if (track) {
          track.innerHTML = '<div class="rail-empty">读取失败</div>';
          bindRail(lib.id);
        }
      });
  }

  /* 海报行翻页：一次滚过可视宽度的 85%，首尾按钮自动置灰。
     滚动条被隐藏了，所以这两个按钮是这一行唯一的翻页入口。 */
  function railScroller(libId) {
    var track = $('#rail-track-' + libId);
    return track ? $('.rail-scroller', track) : null;
  }

  function updateRailNav(libId) {
    var row = $('#rail-body-' + libId);
    if (!row) return;
    var prev = $('.rail-btn.prev', row);
    var next = $('.rail-btn.next', row);
    if (!prev || !next) return;

    var scroller = railScroller(libId);
    if (!scroller) {
      prev.disabled = true;
      next.disabled = true;
      row.classList.add('no-overflow');
      return;
    }

    var max = scroller.scrollWidth - scroller.clientWidth;
    var noOverflow = max <= 2;
    row.classList.toggle('no-overflow', noOverflow);
    prev.disabled = noOverflow || scroller.scrollLeft <= 2;
    next.disabled = noOverflow || scroller.scrollLeft >= max - 2;
  }

  function scrollRail(libId, dir) {
    var scroller = railScroller(libId);
    if (!scroller) return;
    var step = Math.max(Math.round(scroller.clientWidth * 0.85), 240);
    var max = Math.max(0, scroller.scrollWidth - scroller.clientWidth);
    var left = Math.min(Math.max(scroller.scrollLeft + dir * step, 0), max);
    if (scroller.scrollTo) scroller.scrollTo({ left: left, behavior: 'smooth' });
    else scroller.scrollLeft = left;
  }

  function bindRail(libId) {
    var row = $('#rail-body-' + libId);
    if (!row) return;

    if (row.getAttribute('data-nav-bound') !== '1') {
      row.setAttribute('data-nav-bound', '1');
      var prev = $('.rail-btn.prev', row);
      var next = $('.rail-btn.next', row);
      if (prev) prev.addEventListener('click', function () { scrollRail(libId, -1); });
      if (next) next.addEventListener('click', function () { scrollRail(libId, 1); });
    }

    var scroller = railScroller(libId);
    if (scroller && scroller.getAttribute('data-scroll-bound') !== '1') {
      scroller.setAttribute('data-scroll-bound', '1');
      scroller.addEventListener('scroll', function () { updateRailNav(libId); }, { passive: true });
    }
    updateRailNav(libId);
  }

  /* 横排里的一张票：只有封面 + 书名，鼠标悬停才浮出下载键。
     比封面墙的卡片更矮更紧凑 —— 首页一行是"预览"，不是"浏览"。 */
  function railItemHTML(b) {
    return '<a class="rail-item" href="#/book/' + b.id + '" title="' + esc(b.title) + '">' +
        '<span class="rail-cover">' + coverHTML(b, b.title) +
          '<span class="card-badge ' + (b.format === 'pdf' ? 'pdf' : 'epub') + '">' +
            esc(b.format) + '</span>' +
          '<span class="card-tools">' +
            '<span class="card-action" role="button" tabindex="0" data-download="' + b.id + '"' +
              ' title="下载原文件" aria-label="下载原文件">' + svg(ICONS.download) + '</span>' +
          '</span>' +
        '</span>' +
        '<span class="rail-name">' + esc(b.title) + '</span>' +
      '</a>';
  }

  /* ------------------------------------------------------------------ 封面墙 */

  function currentLibraryName() {
    if (state.libraryId === -1) return '全部书籍';
    if (state.libraryId === 0) return '首页';
    var lib = libraryById(state.libraryId);
    return lib ? lib.name : '全部书籍';
  }

  function wallSubtitle() {
    if (state.libraryId > 0) {
      var lib = libraryById(state.libraryId);
      if (lib) return lib.path;
    }
    return '来自 ' + state.libraries.length + ' 个书库';
  }

  function densitySegHTML() {
    var opts = [['s', ICONS.gridS, '小图'], ['m', ICONS.gridM, '中图'], ['l', ICONS.gridL, '大图']];
    return '<div class="segmented icon-only" id="density-seg" role="group" aria-label="封面大小">' +
      opts.map(function (o) {
        return '<button type="button" data-density="' + o[0] + '" title="' + o[2] + '"' +
          ' aria-pressed="false">' + svg(o[1]) + '</button>';
      }).join('') + '</div>';
  }

  /* 密度按钮可能在页头（封面墙）也可能在分区标题里（首页），
     两处共用同一个 id，同一时刻只有一个存在，所以直接用 #density-seg。 */
  function bindDensitySeg() {
    var seg = $('#density-seg');
    if (!seg) return;
    paintDensitySeg();
    $$('button', seg).forEach(function (b) {
      b.addEventListener('click', function () { applyDensity(this.getAttribute('data-density')); });
    });
  }

  function wallQuerySig() {
    return state.libraryId + '|' + state.q + '|' + state.format + '|' + state.sort;
  }

  function renderWallShell() {
    var view = $('#view');

    // 已经在封面墙里：只改文案，不重建 DOM。
    // 搜索 / 筛选 / 换库都会走到这里，重建整块就会"闪一下"。
    if (view.getAttribute('data-view') === 'wall' && $('#grid')) {
      var title = $('#wall-title');
      if (title) title.textContent = currentLibraryName();
      var subEl = $('#wall-sub');
      if (subEl) subEl.textContent = wallSubtitle();
      setTopbar(currentLibraryName(), wallSubtitle());
      setTopbarTools(true);
      paintDensitySeg();
      return;
    }

    // 从详情/书库返回封面墙：网格已经被销毁，先拿上次的查询结果垫上，
    // 这样不会出现一段空白，随后 loadBooks 会把新数据整体替换进来。
    var prefill = (state.wallSig === wallQuerySig() && state.books.length)
      ? state.books.map(cardHTML).join('')
      : '';

    view.setAttribute('data-view', 'wall');
    view.innerHTML =
      '<div class="page-head">' +
        '<div class="grow">' +
          '<h1 class="page-title" id="wall-title">' + esc(currentLibraryName()) + '</h1>' +
          '<div class="page-sub" id="wall-sub">' + esc(wallSubtitle()) + '</div>' +
        '</div>' +
        '<div class="page-tools">' + densitySegHTML() + '</div>' +
      '</div>' +
      '<div id="grid" class="grid">' + prefill + '</div>' +
      '<div id="empty" class="empty u-hidden"></div>' +
      '<div id="feed-foot" class="feed-foot u-hidden"></div>';

    setTopbar(currentLibraryName(), wallSubtitle());
    setTopbarTools(true);
    bindDensitySeg();
  }

  /* 一张卡片 = 封面 + 书名 + 作者。
     系列里的书（如"三体 2"）右上角挂一个卷标签，扫一眼就知道是第几卷。 */
  function cardHTML(b) {
    var seriesBadge = '';
    if (b.series) {
      seriesBadge = '<span class="card-series" title="' + esc(b.series) + '">' +
        (b.volume ? '第 ' + esc(b.volume) + ' 卷' : esc(b.series)) + '</span>';
    }
    return '<a href="#/book/' + b.id + '" class="card rise" data-id="' + b.id + '" title="' + esc(b.title) + '">' +
        '<div class="card-cover">' +
          coverHTML(b, b.title) +
          '<span class="card-badge ' + (b.format === 'pdf' ? 'pdf' : 'epub') + '">' + esc(b.format) + '</span>' +
          seriesBadge +
          '<span class="card-tools">' +
            '<span class="card-action" role="button" tabindex="0" data-download="' + b.id + '"' +
              ' title="下载原文件" aria-label="下载原文件">' + svg(ICONS.download) + '</span>' +
          '</span>' +
        '</div>' +
        '<div class="card-body">' +
          '<div class="card-title u-clamp2">' + esc(b.title) + '</div>' +
          '<div class="card-author u-clamp1">' + esc(b.author || '未知作者') + '</div>' +
        '</div>' +
      '</a>';
  }

  function skeletonHTML(n) {
    var out = '';
    for (var i = 0; i < n; i++) {
      out += '<div class="skeleton-card">' +
        '<div class="sk-cover"></div>' +
        '<div class="sk-line"></div><div class="sk-line short"></div>' +
        '</div>';
    }
    return out;
  }

  function loadBooks(reset) {
    if (state.loading) return Promise.resolve();
    state.loading = true;

    var grid = $('#grid');

    if (reset) {
      state.page = 1;
      state.books = [];
      if (grid) {
        // 网格里已经有卡片（上一次查询的结果或返回时的复用内容）时，
        // 保留它们并轻轻压暗，等新数据到了整体替换 —— 只在真正空白时
        // 才铺骨架屏。这一条直接消掉了搜索 / 筛选 / 换库时的闪烁。
        if (grid.childElementCount) grid.classList.add('is-busy');
        else grid.innerHTML = skeletonHTML(18);
      }
      setFeedFoot('');
      var empty0 = $('#empty');
      if (empty0) empty0.classList.add('u-hidden');
    } else {
      state.page += 1;
    }

    var params = new URLSearchParams();
    if (state.libraryId > 0) params.set('library_id', String(state.libraryId));
    if (state.q) params.set('q', state.q);
    if (state.format) params.set('format', state.format);
    var sortParts = state.sort.split(':');
    params.set('sort', sortParts[0]);
    params.set('order', sortParts[1] || 'asc');
    params.set('page', String(state.page));
    params.set('page_size', String(state.pageSize));

    return api('/api/books?' + params.toString()).then(function (data) {
      state.total = data.total;
      state.pages = data.pages;
      state.lastElapsed = data.elapsed_ms || 0;
      state.books = state.books.concat(data.books || []);

      var cards = (data.books || []).map(cardHTML).join('');
      if (reset) {
        // 整块只改一次，只触发一次重绘 —— 不做"先清空再逐条插入"。
        if (grid) grid.innerHTML = cards;
        state.wallSig = wallQuerySig();
      } else if (grid) {
        grid.insertAdjacentHTML('beforeend', cards);
      }

      var sub = $('#wall-sub');
      if (sub) {
        sub.textContent = data.total + ' 本' + (state.q || state.format ? '（已筛选）' : '') +
          (state.libraryId > 0 ? '，' + wallSubtitle() : '');
      }


      renderEmptyState();
      closeBootGate();
      renderFeedFoot(data);
    }).catch(function (err) {
      toast(err.message, 'err');
      if (reset && grid && !grid.childElementCount) grid.innerHTML = '';
      renderEmptyState();
    }).then(function () {
      state.loading = false;
      if (grid) grid.classList.remove('is-busy');
    });
  }

  /* 页脚：滚动到底自动加载下一页；没有更多了就写一行"到头了"。
     以前这里是一个"加载更多"按钮，滚动浏览时得反复去点它。 */
  function setFeedFoot(html) {
    var foot = $('#feed-foot');
    if (!foot) return;
    if (!html) {
      foot.classList.add('u-hidden');
      foot.innerHTML = '';
      return;
    }
    foot.innerHTML = html;
    foot.classList.remove('u-hidden');
  }

  function renderFeedFoot(data) {
    var n = (state.books || []).length;
    if (data.has_more) {
      setFeedFoot('<span>已显示 ' + n + ' / ' + data.total + ' 本</span>' +
        '<button type="button" class="btn btn-sm" id="more-btn">加载更多</button>');
      var btn = $('#more-btn');
      if (btn) btn.addEventListener('click', function () { loadBooks(false); });
      return;
    }
    if (n === 0) { setFeedFoot(''); return; }
    var ms = state.lastElapsed ? (' · 用时 ' + state.lastElapsed + ' ms') : '';
    setFeedFoot('<span>共 ' + data.total + ' 本' + ms + '</span>');
  }

  function renderEmptyState() {
    var empty = $('#empty');
    if (!empty) return;

    if (state.total > 0) { empty.classList.add('u-hidden'); return; }

    empty.classList.remove('u-hidden');
    if (state.libraries.length === 0) {
      empty.innerHTML =
        '<div class="empty-mark">' + iconHTML('builtin:library') + '</div>' +
        '<h2>先创建一个书库</h2>' +
        '<p>书库对应 NAS 上的一个目录。创建后 Libra 会自动递归扫描其中的 EPUB / PDF，' +
        '提取书名、作者与封面，然后在这里铺成一面书架。</p>' +
        '<a href="#/libraries" class="btn btn-primary">' + svg(ICONS.plus) + '新建书库</a>';
      return;
    }

    empty.innerHTML =
      '<div class="empty-mark">' + svg(ICONS.search) + '</div>' +
      '<h2>' + (state.q || state.format ? '没有匹配的书籍' : '这个书库还是空的') + '</h2>' +
      '<p>' + (state.q || state.format
        ? '换个关键词，或者把格式筛选切回「全部」试试。'
        : '确认目录里确实有 EPUB / PDF 文件，然后对这座书库执行一次重新扫描。') + '</p>';
  }

  /* ------------------------------------------------------------------ 书籍详情 */

  function renderDetail(id) {
    var view = $('#view');
    state.detailId = id;
    stopHeroRotation();
    // 详情页顶栏只要书名与作者：排序、深浅色切换、标记墙都不再显示
    setTopbarTools(false);
    view.setAttribute('data-view', 'detail');

    // 详情接口在局域网里通常几毫秒就回来。原先是一进去就先把内容换成
    // 骨架屏，于是"进详情"必然闪一下。现在只有明显慢（>220ms）时才退回
    // 骨架屏，正常情况直接由封面墙整体切换成详情。
    var slowTimer = setTimeout(function () {
      if (view.getAttribute('data-view') !== 'detail' || state.detailId !== id) return;
      setTopbar('书籍详情', '');
      view.innerHTML = '<div class="page-head"><div class="grow">' +
        '<div class="sk-line" style="width:180px"></div></div></div>' +
        '<div class="detail-hero"><div class="detail-grid">' +
        '<div class="detail-cover"></div>' +
        '<div class="detail-main"><div class="sk-line" style="width:60%"></div>' +
        '<div class="sk-line" style="width:35%"></div></div></div></div>';
    }, 220);

    api('/api/books/' + id).then(function (b) {
      clearTimeout(slowTimer);
      if (state.detailId !== id) return;
      var isPDF = b.format === 'pdf';
      var glow = b.cover_url ? ' style="background-image:url(&quot;' + esc(b.cover_url) + '&quot;)"' : '';

      function spec(label, value, opts2) {
        opts2 = opts2 || {};
        return '<div class="spec' + (opts2.full ? ' full' : '') + '">' +
          '<dt>' + label + '</dt>' +
          '<dd' + (opts2.mono ? ' class="mono"' : '') + '>' + value + '</dd></div>';
      }

      var html =
        '<a href="javascript:history.back()" class="back-link">' + svg(ICONS.back) + '返回</a>' +

        '<div class="detail-hero rise">' +
          '<div class="detail-glow"' + glow + '></div>' +
          '<div class="detail-grid">' +

            '<div class="detail-cover">' + coverHTML(b, b.title, { original: true }) + '</div>' +

            '<div class="detail-main">' +
              '<h1 class="detail-title">' + esc(b.title) + '</h1>' +
              '<div class="detail-author">' + esc(b.author || '未知作者') + '</div>' +

              '<div class="detail-tags">' +
                '<span class="tag ' + (isPDF ? '' : 'accent') + '">' + esc(b.format.toUpperCase()) + '</span>' +
                (b.series
                  ? '<span class="tag accent">' + esc(b.series) +
                      (b.volume ? ' · 第 ' + esc(b.volume) + ' 卷' : '') + '</span>'
                  : '') +
                '<span class="tag">' + esc(b.size_human) + '</span>' +
                '<span class="tag">' + iconHTML('builtin:library') + esc(b.library_name) + '</span>' +
                (b.meta_locked
                  ? '<span class="tag">' + svg(ICONS.pencil) + '元数据已手动编辑</span>'
                  : '') +
                (b.file_exists === false
                  ? '<span class="tag danger">' + svg(ICONS.alert) + '原文件已不在磁盘上</span>'
                  : '<span class="tag ok">' + svg(ICONS.check) + '可下载</span>') +
              '</div>' +

              '<div class="specs">' +
                spec('文件格式', esc(b.format.toUpperCase())) +
                spec('文件大小', esc(b.size_human) + ' <span class="dim">' + esc(String(b.size)) + ' 字节</span>') +
                spec('所属书库', '<a href="#/lib/' + b.library_id + '" class="link">' + esc(b.library_name) + '</a>') +
                spec('入库时间', esc(fmtTime(b.created_at))) +
                spec('原始文件名', esc(b.filename), { full: true, mono: true }) +
              '</div>' +

              /* 详情页只保留"下载原文件"。重新扫描统一在书库管理页操作，
                 免得在看书的时候误触一次全库扫描。管理员额外可以编辑元数据 / 移除书籍。 */
              '<div class="detail-actions">' +
                '<a href="/download/' + b.id + '" class="btn btn-download btn-lg">' +
                  svg(ICONS.download) + '下载原文件</a>' +
                (IS_ADMIN
                  ? '<button type="button" id="book-edit-meta" class="btn btn-lg">' +
                      svg(ICONS.pencil) + '编辑元数据</button>' +
                    '<button type="button" id="book-delete" class="btn btn-lg btn-danger">' +
                      svg(ICONS.trash) + '删除书籍</button>'
                  : '') +
              '</div>' +
            '</div>' +

          '</div>' +
        '</div>' +
        '<div class="detail-more" id="detail-more"></div>';

      view.innerHTML = html;
      setTopbar(b.title, b.author || '');
      closeBootGate();

      renderDetailMore(b);

      if (IS_ADMIN) {
        var editMeta = $('#book-edit-meta');
        if (editMeta) editMeta.addEventListener('click', function () { openBookMetaForm(b); });
        var delBook = $('#book-delete');
        if (delBook) delBook.addEventListener('click', function () { confirmDeleteBook(b); });
      }
    }).catch(function (err) {
      clearTimeout(slowTimer);
      if (state.detailId !== id) return;
      var errHTML = '<div class="empty"><div class="empty-mark">' + svg(ICONS.alert) + '</div>' +
        '<h2>打不开这本书</h2><p>' + esc(err.message) + '</p>' +
        '<a href="#/" class="btn">返回书库</a></div>';
      setTopbar('书籍详情', '');
      view.innerHTML = errHTML;
    });
  }

  /* 管理员：编辑书名 / 作者。保存后 meta_locked=1，扫描不会覆盖。 */
  function openBookMetaForm(b) {
    var modal = openModal(
      '<form id="book-meta-form">' +
        '<div class="modal-head">' +
          '<h3>编辑书籍元数据</h3>' +
          '<p>' + esc(b.format.toUpperCase()) + ' · ' + esc(b.filename) + '</p>' +
        '</div>' +
        '<div class="modal-body">' +
          '<div class="field">' +
            '<label for="bm-title">书名</label>' +
            '<input id="bm-title" type="text" class="input" maxlength="500" required ' +
              'value="' + esc(b.title) + '">' +
          '</div>' +
          '<div class="field">' +
            '<label for="bm-author">作者</label>' +
            '<input id="bm-author" type="text" class="input" maxlength="300" ' +
              'value="' + esc(b.author || '') + '" placeholder="未知作者">' +
          '</div>' +
          '<div class="alert info">' + svg(ICONS.info) +
            '<span>保存后会同时写入 EPUB / PDF 源文件，并锁定数据库元数据；需要书库目录具有写权限。</span></div>' +
          '<div id="bm-error" class="alert error u-hidden" style="margin-top:14px"></div>' +
        '</div>' +
        '<div class="modal-foot">' +
          '<button type="button" data-close class="btn">取消</button>' +
          '<button type="submit" id="bm-save" class="btn btn-primary">' + svg(ICONS.check) + '保存</button>' +
        '</div>' +
      '</form>'
    );

    var form = page.$('#book-meta-form');
    var title = page.$('#bm-title');
    var author = page.$('#bm-author');
    var errBox = page.$('#bm-error');
    var save = page.$('#bm-save');

    form.addEventListener('submit', function (ev) {
      ev.preventDefault();
      errBox.classList.add('u-hidden');
      var nextTitle = title.value.trim();
      if (!nextTitle) {
        errBox.innerHTML = svg(ICONS.alert) + '<span>书名不能为空</span>';
        errBox.classList.remove('u-hidden');
        title.focus();
        return;
      }
      save.disabled = true;
      save.textContent = '保存中…';
      api('/api/books/' + b.id + '/metadata', {
        method: 'PUT',
        body: { title: nextTitle, author: author.value.trim() }
      }).then(function () {
        modal.close();
        toast('元数据已保存', 'ok');
        state.featured = null;
        state.libCovers = {};
        renderDetail(b.id);
      }).catch(function (err) {
        save.disabled = false;
        save.innerHTML = svg(ICONS.check) + '保存';
        errBox.innerHTML = svg(ICONS.alert) + '<span>' + esc(err.message) + '</span>';
        errBox.classList.remove('u-hidden');
      });
    });
  }

  /* 管理员：从书库移除书籍。软删除，保留 NAS 上的原始文件。 */
  function confirmDeleteBook(b) {
    var modal = openModal(
      '<div class="modal-head">' +
        '<h3>删除书籍</h3>' +
        '<p>确定要把「<b>' + esc(b.title) + '</b>」从书库中移除吗？</p>' +
      '</div>' +
      '<div class="modal-body">' +
        '<div class="alert error">' + svg(ICONS.alert) +
          '<span>只会移除 Libra 中的书籍记录与封面，<b>不会删除 NAS 上的原始文件</b>。' +
          '移除后重新扫描也不会重新添加。</span></div>' +
      '</div>' +
      '<div class="modal-foot">' +
        '<button type="button" data-close class="btn">取消</button>' +
        '<button type="button" id="book-del-confirm" class="btn btn-primary danger-solid">' +
          svg(ICONS.trash) + '确认删除</button>' +
      '</div>'
    );

    var btn = page.$('#book-del-confirm');
    btn.addEventListener('click', function () {
      btn.disabled = true;
      btn.textContent = '删除中…';
      api('/api/books/' + b.id, { method: 'DELETE' }).then(function () {
        modal.close();
        toast('书籍已从书库移除', 'ok');
        state.featured = null;
        state.libCovers = {};
        loadLibraries().catch(function () {}).then(function () {
          location.hash = '#/lib/' + b.library_id;
        });
      }).catch(function (err) {
        btn.disabled = false;
        btn.innerHTML = svg(ICONS.trash) + '确认删除';
        toast(err.message, 'err');
      });
    });
  }

  /* 详情页底部：同一系列的其他卷，或同一书库里的其他书。
     这块是详情页最实用的一段 —— 看完一本往往想看下一本。 */
  function renderDetailMore(b) {
    var box = $('#detail-more');
    if (!box) return;

    function paint(title, books) {
      if (!books.length) return;
      box.innerHTML =
        '<div class="section-head"><h2>' + esc(title) + '</h2>' +
          '<span class="grow"></span>' +
          '<a class="more" href="#/lib/' + b.library_id + '">查看全部' + svg(ICONS.chevron) + '</a>' +
        '</div>' +
        '<div class="grid">' + books.map(cardHTML).join('') + '</div>';
    }

    function fallback() {
      return api('/api/books?library_id=' + b.library_id + '&page_size=13&sort=title').then(function (d2) {
        var more = (d2.books || []).filter(function (x) { return x.id !== b.id; });
        paint('「' + b.library_name + '」里的其他书', more.slice(0, 12));
      });
    }

    if (!b.series) {
      fallback().catch(function () { /* 推荐失败不影响详情 */ });
      return;
    }

    var params = new URLSearchParams();
    params.set('library_id', String(b.library_id));
    params.set('page_size', '12');
    params.set('sort', 'title');
    params.set('q', b.series);

    api('/api/books?' + params.toString()).then(function (d) {
      var others = (d.books || []).filter(function (x) { return x.id !== b.id; });
      if (others.length) {
        paint('同系列 · ' + b.series, others.slice(0, 12));
        return;
      }
      return fallback();
    }).catch(function () { /* 推荐失败不影响详情 */ });
  }

  /* ------------------------------------------------------------------ 管理后台 */

  function renderAdminView() {
    var view = $('#view');
    stopHeroRotation();
    view.setAttribute('data-view', 'admin');
    setTopbarTools(false);
    view.innerHTML =
      '<div class="page-head">' +
        '<div class="grow">' +
          '<h1 class="page-title">管理后台</h1>' +
          '<div class="page-sub">系统概览、注册设置与管理员工具</div>' +
        '</div>' +
      '</div>' +
      '<div class="admin-grid">' +
        '<section class="admin-card">' +
          '<div class="admin-card-head">' +
            '<div>' +
              '<h2>开放注册</h2>' +
              '<p>关闭后只有管理员能在「账号管理」里创建账号。</p>' +
            '</div>' +
            '<label class="switch" title="开放注册">' +
              '<input id="allow-register" type="checkbox" disabled aria-label="开放注册">' +
              '<span class="switch-track"></span>' +
            '</label>' +
          '</div>' +
          '<div id="register-state" class="hint">读取中…</div>' +
        '</section>' +

        '<a class="admin-card admin-link" href="#/admin/featured">' +
          '<div class="admin-link-head"><span class="admin-icon">' + svg(ICONS.dice) + '</span>' +
            '<span class="admin-arrow">' + svg(ICONS.chevron) + '</span></div>' +
          '<h2>横幅数据源</h2>' +
          '<p>首页轮播用哪些书：每日推荐书单，或从指定书库里随机取。</p>' +
        '</a>' +

        '<a class="admin-card admin-link" href="#/libraries">' +
          '<div class="admin-link-head"><span class="admin-icon">' + iconHTML('builtin:library') + '</span>' +
            '<span class="admin-arrow">' + svg(ICONS.chevron) + '</span></div>' +
          '<h2>书库管理</h2>' +
          '<p>新建书库、修改目录、上传封面、触发扫描或删除书库。</p>' +
        '</a>' +

        '<a class="admin-card admin-link" href="#/users">' +
          '<div class="admin-link-head"><span class="admin-icon">' + iconHTML('builtin:user') + '</span>' +
            '<span class="admin-arrow">' + svg(ICONS.chevron) + '</span></div>' +
          '<h2>账号管理</h2>' +
          '<p>创建账号、调整角色、重置密码、删除账号。</p>' +
        '</a>' +

        '<section class="admin-card" id="admin-info">' +
          '<h2>系统信息</h2>' +
          '<p>正在读取…</p>' +
        '</section>' +
      '</div>';

    setTopbar('管理后台', '系统设置与管理员工具');
    closeBootGate();

    var toggle = $('#allow-register');
    var stateNode = $('#register-state');

    function paintRegisterState(on) {
      if (toggle) {
        toggle.checked = on;
        toggle.disabled = false;
      }
      if (stateNode) {
        stateNode.innerHTML = on
          ? '<span class="tag ok">已开放</span><span>任何访客都可以注册普通成员账号。</span>'
          : '<span class="tag">已关闭</span><span>注册入口隐藏，仅管理员可创建账号。</span>';
      }
    }

    Promise.all([api('/api/admin/settings'), api('/api/status')]).then(function (res) {
      var settings = res[0] || {};
      var status = res[1] || {};
      paintRegisterState(!!settings.allow_register);

      var st = status.stats || {};
      var info = $('#admin-info');
      if (!info) return;
      info.innerHTML =
        '<h2>系统信息</h2>' +
        '<div class="admin-stats">' +
          '<div class="admin-stat"><span>版本</span><b>v' + esc(status.version || '—') + '</b></div>' +
          '<div class="admin-stat"><span>书库</span><b>' + (status.library_count || 0) + '</b></div>' +
          '<div class="admin-stat"><span>书籍</span><b>' + (st.total || 0) + '</b></div>' +
          '<div class="admin-stat"><span>EPUB / PDF</span><b>' + (st.epub || 0) + ' / ' + (st.pdf || 0) + '</b></div>' +
          '<div class="admin-stat"><span>占用体积</span><b>' + fmtBytes(st.total_size || 0) + '</b></div>' +
          '<div class="admin-stat"><span>PDF 封面</span><b>' + (status.pdf_support ? '可用' : '未安装') + '</b></div>' +
        '</div>' +
        '<div class="admin-datadir"><span>数据目录</span><code>' + esc(status.data_dir || '—') + '</code></div>';

      if (toggle) {
        toggle.addEventListener('change', function () {
          var next = this.checked;
          toggle.disabled = true;
          api('/api/admin/settings', { method: 'PUT', body: { allow_register: next } }).then(function (d) {
            paintRegisterState(!!d.allow_register);
            toast(d.allow_register ? '已开放注册' : '已关闭注册', 'ok');
          }).catch(function (e) {
            paintRegisterState(!next);
            toast(e.message, 'err');
          });
        });
      }
    }).catch(function (e) {
      if (stateNode) stateNode.innerHTML = '<span class="tag danger">读取失败</span><span>' + esc(e.message) + '</span>';
    });
  }

  /* ------------------------------------------------------------------ 书库管理 */

  function renderLibrariesView() {
    var view = $('#view');
    stopHeroRotation();
    var totalBooks = state.libraries.reduce(function (a, l) { return a + (l.book_count || 0); }, 0);

    var html =
      '<div class="page-head">' +
        '<div class="grow">' +
          '<h1 class="page-title">书库</h1>' +
          '<div class="page-sub">每座书库对应磁盘上的一个目录，独立扫描与统计 · 当前共 ' +
            state.libraries.length + ' 座书库、' + totalBooks + ' 本藏书</div>' +
        '</div>' +
        (IS_ADMIN
          ? '<div class="page-tools">' +
              '<button id="new-lib" class="btn btn-primary">' + svg(ICONS.plus) + '新建书库</button>' +
            '</div>'
          : '') +
      '</div>';

    if (!state.libraries.length) {
      html += '<div class="empty"><div class="empty-mark">' + iconHTML('builtin:folder') + '</div>' +
        '<h2>还没有书库</h2><p>' +
        (IS_ADMIN
          ? '把一个装满电子书的目录挂进来，作为第一座书库。'
          : '管理员还没有添加任何书库。') +
        '</p></div>';
    } else {
      html += '<div class="lib-list">' + state.libraries.map(libraryCardHTML).join('') + '</div>';
    }

    view.setAttribute('data-view', 'libraries');
    view.innerHTML = html;
    setTopbar('书库', state.libraries.length + ' 座书库');
    setTopbarTools(false);
    closeBootGate();

    var newBtn = $('#new-lib');
    if (newBtn) newBtn.addEventListener('click', function () { openLibraryForm(null); });

    $$('[data-action]', view).forEach(function (btn) {
      btn.addEventListener('click', function () {
        var action = this.getAttribute('data-action');
        var lib = libraryById(Number(this.getAttribute('data-id')));
        if (!lib) return;

        if (action === 'scan') triggerScan(lib.id).catch(function (e) { toast(e.message, 'err'); });
        else if (action === 'edit') openLibraryForm(lib);
        else if (action === 'delete') confirmDeleteLibrary(lib);
      });
    });
  }

  function libraryCardHTML(lib) {
    var scanning = !!lib.scanning;
    var st = lib.scan || null;

    var progress = '';
    if (scanning && st) {
      var pct = st.total > 0 ? Math.round((st.processed / st.total) * 100) : 4;
      progress =
        '<div class="lib-progress">' +
          '<div class="row"><span>正在扫描…</span><span>' +
            (st.total > 0 ? st.processed + ' / ' + st.total : '统计目录中') + '</span></div>' +
          '<div class="track"><div class="fill" style="width:' + pct + '%"></div></div>' +
        '</div>';
    }

    var stateTag;
    if (scanning) {
      stateTag = '<span class="tag accent">' + svg('<path d="M12 3a9 9 0 1 0 9 9"/>') + '扫描中</span>';
    } else if (lib.last_scan_state === 'error') {
      stateTag = '<span class="tag danger">' + svg(ICONS.alert) + '上次失败</span>';
    } else if (lib.last_scan_state === 'done') {
      stateTag = '<span class="tag ok">' + svg(ICONS.check) + '已扫描</span>';
    } else {
      stateTag = '<span class="tag">未扫描</span>';
    }

    return '<div class="lib-card rise">' +
        (lib.cover_url
          ? '<div class="lib-cover-thumb"><img src="' + esc(lib.cover_thumb_url || lib.cover_url) + '" alt=""></div>'
          : '<div class="lib-icon" style="' + tintStyle(lib.name) + '">' + iconHTML(lib.icon) + '</div>') +
        '<div class="lib-body">' +
          '<div class="lib-name"><h2>' + esc(lib.name) + '</h2>' + stateTag + '</div>' +
          '<div class="lib-path">' + esc(lib.path) + '</div>' +
          '<div class="lib-meta">' +
            '<span>书籍 <b>' + (lib.book_count || 0) + '</b></span>' +
            '<span>EPUB <b>' + (lib.epub_count || 0) + '</b></span>' +
            '<span>PDF <b>' + (lib.pdf_count || 0) + '</b></span>' +
            '<span>上次扫描 ' + fmtTime(lib.last_scan_at) + '</span>' +
          '</div>' +
          progress +
        '</div>' +
        '<div class="lib-actions">' +
          '<a class="btn btn-sm" href="#/lib/' + lib.id + '">' +
            iconHTML('builtin:books') + '浏览书架</a>' +
          (IS_ADMIN
            ? '<button class="btn btn-sm" data-action="scan" data-id="' + lib.id + '"' +
                (scanning ? ' disabled' : '') + '>' + svg(ICONS.refresh) + '扫描</button>' +
              '<button class="btn btn-sm" data-action="edit" data-id="' + lib.id + '">' +
                svg(ICONS.pencil) + '编辑</button>' +
              '<button class="btn btn-sm btn-danger" data-action="delete" data-id="' + lib.id + '">' +
                svg(ICONS.trash) + '删除</button>'
            : '') +
        '</div>' +
      '</div>';
  }

  function confirmDeleteLibrary(lib) {
    var modal = openModal(
      '<div class="modal-head">' +
        '<h3>删除书库</h3>' +
        '<p>将要删除「<b>' + esc(lib.name) + '</b>」及其 <b>' + (lib.book_count || 0) +
          '</b> 条书籍记录与已生成的封面文件。</p>' +
      '</div>' +
      '<div class="modal-body">' +
        '<div class="alert info">' + svg(ICONS.info) +
          '<span>磁盘上的原始文件不会被删除，随时可以重新添加这个目录。</span></div>' +
      '</div>' +
      '<div class="modal-foot">' +
        '<button type="button" data-close class="btn">取消</button>' +
        '<button id="confirm-del" class="btn btn-primary danger-solid">' +
          svg(ICONS.trash) + '确认删除</button>' +
      '</div>'
    );

    var btn = page.$('#confirm-del');
    btn.addEventListener('click', function () {
      btn.disabled = true;
      btn.textContent = '删除中…';
      api('/api/libraries/' + lib.id, { method: 'DELETE' }).then(function () {
        modal.close();
        toast('书库已删除', 'ok');
        state.featured = null;
        state.libCovers = {};
        if (state.libraryId === lib.id) { state.libraryId = 0; location.hash = '#/'; }
        return loadLibraries().then(renderLibrariesView);
      }).catch(function (err) {
        btn.disabled = false;
        btn.innerHTML = svg(ICONS.trash) + '确认删除';
        toast(err.message, 'err');
      });
    });
  }

  /* ------------------------------------------------------------------ 书库表单 */

  function openLibraryForm(lib) {
    var isEdit = !!lib;
    var icon = lib ? lib.icon : DEFAULT_ICON;
    var coverURL = lib ? (lib.cover_url || '') : '';

    var modal = openModal(
      '<form id="lib-form">' +
        '<div class="modal-head">' +
          '<h3>' + (isEdit ? '编辑书库' : '新建书库') + '</h3>' +
          '<p>' + (isEdit
            ? '修改名称或图标不会影响已扫描的书籍；改动目录后需要重新扫描。'
            : '指定一个磁盘目录，Libra 会自动递归扫描其中的 EPUB / PDF。') + '</p>' +
        '</div>' +

        '<div class="modal-body">' +
          '<div class="field">' +
            '<label for="lib-name">名称</label>' +
            '<input id="lib-name" type="text" class="input" maxlength="64" required ' +
              'placeholder="例如：技术书籍、小说、杂志" value="' + esc(lib ? lib.name : '') + '">' +
          '</div>' +

          '<div class="field">' +
            '<label for="lib-path">目录路径</label>' +
            '<div class="form-row">' +
              '<input id="lib-path" type="text" class="input mono" required placeholder="/data/books" ' +
                'value="' + esc(lib ? lib.path : '') + '">' +
              '<button type="button" id="lib-browse" class="btn">' + svg(ICONS.folderOpen) + '浏览</button>' +
            '</div>' +
            '<div class="hint">Libra 只读取该目录，不会修改或删除其中的任何原始文件。</div>' +
          '</div>' +

          (isEdit
            ? '<div class="field">' +
                '<label>书库封面</label>' +
                '<div class="lib-cover-edit">' +
                  '<div id="lib-cover-preview" class="lib-cover-preview"></div>' +
                  '<div class="lib-cover-side">' +
                    '<input id="lib-cover-file" type="file" accept="image/png,image/jpeg,image/webp,image/gif" class="u-hidden">' +
                    '<div class="lib-cover-buttons">' +
                      '<button type="button" id="lib-cover-upload" class="btn btn-sm">' + svg(ICONS.image) + '上传封面</button>' +
                      '<button type="button" id="lib-cover-reset" class="btn btn-sm" disabled>恢复自动拼图</button>' +
                    '</div>' +
                    '<div class="hint">建议使用 16:9 横图。不上传时自动取最近几本书的封面斜向拼在一起。</div>' +
                  '</div>' +
                '</div>' +
              '</div>'
            : '') +

          '<div class="field">' +
            '<label>图标</label>' +
            '<div id="icon-grid" class="icon-grid"></div>' +
            '<div style="display:flex;align-items:center;gap:9px;margin-top:9px">' +
              '<input id="icon-file" type="file" accept="image/*" class="u-hidden">' +
              '<button type="button" id="icon-upload" class="btn btn-sm">' + svg(ICONS.image) + '上传图标</button>' +
              '<span id="icon-upload-name" style="font-size:11.5px;color:var(--text-3)" class="u-clamp1"></span>' +
            '</div>' +
          '</div>' +
        '</div>' +

        '<div id="lib-error" class="alert error u-hidden" style="margin:0 24px 18px"></div>' +

        '<div class="modal-foot">' +
          '<button type="button" data-close class="btn">取消</button>' +
          '<button type="submit" id="lib-submit" class="btn btn-primary">' +
            (isEdit ? '保存' : '创建并扫描') + '</button>' +
        '</div>' +
      '</form>', { wide: true }
    );

    function paintCoverPreview() {
      var box = page.$('#lib-cover-preview');
      if (!box) return;
      box.innerHTML = coverURL
        ? '<img src="' + esc(coverURL) + '" alt="">'
        : '<span class="lib-cover-auto">' + iconHTML(icon) + '<span>自动拼图</span></span>';
      var up = page.$('#lib-cover-upload');
      if (up) up.innerHTML = svg(ICONS.image) + (coverURL ? '更换封面' : '上传封面');
      var reset = page.$('#lib-cover-reset');
      if (reset) {
        reset.disabled = !coverURL;
        reset.textContent = '恢复自动拼图';
      }
    }
    paintCoverPreview();

    function refreshIconTiles(uploaded, builtin) {
      var grid = page.$('#icon-grid');
      var tiles = (builtin || []).map(function (name) {
        var id = 'builtin:' + name;
        return '<button type="button" data-icon="' + id + '" title="' + esc(name) + '"' +
          ' class="icon-tile' + (icon === id ? ' active' : '') + '">' + iconHTML(id) + '</button>';
      }).join('');

      tiles += (uploaded || []).map(function (u) {
        return '<button type="button" data-icon="' + esc(u.id) + '" title="已上传"' +
          ' class="icon-tile' + (icon === u.id ? ' active' : '') + '">' +
          '<img src="' + esc(u.url) + '" alt="">' + '</button>';
      }).join('');

      grid.innerHTML = tiles;
      $$('[data-icon]', grid).forEach(function (btn) {
        btn.addEventListener('click', function () {
          icon = this.getAttribute('data-icon');
          $$('[data-icon]', grid).forEach(function (b) { b.classList.remove('active'); });
          this.classList.add('active');
          paintCoverPreview();
        });
      });
    }

    state.iconCache = state.iconCache || api('/api/icons');
    state.iconCache.then(function (d) {
      refreshIconTiles(d.uploaded, d.builtin);
    }).catch(function () {
      refreshIconTiles([], Object.keys(ICONS));
    });

    page.$('#icon-upload').addEventListener('click', function () { page.$('#icon-file').click(); });
    page.$('#icon-file').addEventListener('change', function () {
      if (!this.files || !this.files[0]) return;
      var fd = new FormData();
      fd.append('file', this.files[0]);
      apiUpload('/api/icons', fd).then(function (d) {
        icon = d.id;
        page.$('#icon-upload-name').textContent = '已上传：' + d.name;
        state.iconCache = api('/api/icons');
        return state.iconCache;
      }).then(function (d2) {
        refreshIconTiles(d2.uploaded, d2.builtin);
      }).catch(function (err) {
        toast(err.message, 'err');
      });
    });

    // 书库封面：上传后立即生效，表单的其它字段照常保存
    var coverUpload = page.$('#lib-cover-upload');
    if (coverUpload) {
      var coverFile = page.$('#lib-cover-file');
      var coverReset = page.$('#lib-cover-reset');

      coverUpload.addEventListener('click', function () { coverFile.click(); });
      coverFile.addEventListener('change', function () {
        if (!this.files || !this.files[0]) return;
        var fd = new FormData();
        fd.append('file', this.files[0]);
        var self = this;
        coverUpload.disabled = true;
        coverUpload.textContent = '上传中…';
        apiUpload('/api/libraries/' + lib.id + '/cover', fd).then(function (d) {
          coverURL = d.cover_url || '';
          var live = libraryById(lib.id);
          if (live) live.cover_url = coverURL;
          toast('书库封面已更新', 'ok');
        }).catch(function (err) {
          toast(err.message, 'err');
        }).then(function () {
          coverUpload.disabled = false;
          self.value = '';
          paintCoverPreview();
        });
      });

      coverReset.addEventListener('click', function () {
        if (!coverURL) return;
        coverReset.disabled = true;
        api('/api/libraries/' + lib.id + '/cover', { method: 'DELETE' }).then(function () {
          coverURL = '';
          var live = libraryById(lib.id);
          if (live) live.cover_url = '';
          paintCoverPreview();
          toast('已恢复自动拼图', 'ok');
        }).catch(function (err) {
          toast(err.message, 'err');
          paintCoverPreview();
        });
      });
    }

    page.$('#lib-browse').addEventListener('click', function () {
      openDirPicker(page.$('#lib-path').value.trim(), function (picked) {
        page.$('#lib-path').value = picked;
      });
    });

    page.$('#lib-form').addEventListener('submit', function (ev) {
      ev.preventDefault();
      var name = page.$('#lib-name').value.trim();
      var path = page.$('#lib-path').value.trim();
      var err = page.$('#lib-error');

      function fail(msg) {
        err.innerHTML = svg(ICONS.alert) + '<span>' + esc(msg) + '</span>';
        err.classList.remove('u-hidden');
      }

      if (!name) { fail('请填写书库名称'); return; }
      if (!path) { fail('请选择目录路径'); return; }
      err.classList.add('u-hidden');

      var btn = page.$('#lib-submit');
      btn.disabled = true;
      btn.textContent = isEdit ? '保存中…' : '创建中…';

      var payload = { name: name, path: path, icon: icon };
      var req = isEdit
        ? api('/api/libraries/' + lib.id, { method: 'PUT', body: payload })
        : api('/api/libraries', { method: 'POST', body: payload });

      req.then(function (data) {
        modal.close();
        toast(isEdit ? '书库已更新' : '书库已创建，正在扫描…', 'ok');
        state.featured = null;
        state.libCovers = {};
        var libId = data.library ? data.library.id : (lib ? lib.id : 0);
        if (data.scan_started || data.rescan_started) {
          if (libId && state.scanning.indexOf(libId) === -1) state.scanning.push(libId);
          startPolling();
        }
        return loadLibraries().then(function () {
          var r = parseHash();
          if (r.name === 'libraries') renderLibrariesView();
          else if (r.name === 'home') renderHome(true);
          else loadBooks(true);
        });
      }).catch(function (e) {
        fail(e.message);
        btn.disabled = false;
        btn.textContent = isEdit ? '保存' : '创建并扫描';
      });
    });
  }

  /* ------------------------------------------------------------------ 账号管理 */

  // 只有管理员能进来；非管理员手敲 #/users 会被弹回全部书籍。
  // （弹回封面墙而不是首页，是因为"想看某个账号、结果被扔到家门口"更让人困惑。）
  function renderUsersView() {
    if (!IS_ADMIN) { location.hash = '#/'; return; }
    stopHeroRotation();

    var view = $('#view');

    var html =
      '<div class="page-head">' +
        '<div class="grow">' +
          '<h1 class="page-title">账号管理</h1>' +
          '<div class="page-sub" id="users-sub">正在读取…</div>' +
        '</div>' +
        '<div class="page-tools">' +
          '<button id="new-user" class="btn btn-primary">' + svg(ICONS.plus) + '新建账号</button>' +
        '</div>' +
      '</div>' +
      '<div id="users-list" class="lib-list"></div>';

    view.setAttribute('data-view', 'users');
    view.innerHTML = html;
    setTopbar('账号管理', '');
    setTopbarTools(false);
    closeBootGate();

    $('#new-user').addEventListener('click', function () { openUserForm(); });
    loadUsers();
  }

  function loadUsers() {
    return api('/api/users').then(function (d) {
      renderUserList(d.users || []);
    }).catch(function (e) {
      toast(e.message, 'err');
      var sub = $('#users-sub');
      if (sub) sub.textContent = '读取账号失败：' + e.message;
    });
  }

  function renderUserList(users) {
    var list = $('#users-list');
    if (!list) return;

    var admins = users.filter(function (u) { return u.is_admin; }).length;
    var sub = $('#users-sub');
    if (sub) {
      sub.textContent = '共 ' + users.length + ' 个账号，管理员 ' + admins +
        ' 个。管理员可管理书库与账号，普通成员只能浏览和下载';
      setTopbar('账号', users.length + ' 个账号');
    }

    if (!users.length) {
      list.innerHTML = '<div class="empty"><div class="empty-mark">' +
        iconHTML('builtin:user') + '</div><h2>还没有账号</h2></div>';
      return;
    }

    list.innerHTML = users.map(function (u) {
      // 最后一个管理员不能再被降级或删除，否则系统会永久失去管理入口。
      var lastAdmin = u.is_admin && admins <= 1;
      var guard = lastAdmin ? ' disabled title="至少要保留一个管理员"' : '';

      var roleTag = u.is_admin
        ? '<span class="tag accent">' + svg(ICONS.shield) + '管理员</span>'
        : '<span class="tag">普通成员</span>';

      var actions;
      if (u.is_self) {
        actions =
          '<span class="tag ok">' + svg(ICONS.check) + '当前登录</span>' +
          '<button class="btn btn-sm" data-uaction="password" data-id="' + u.id +
            '" data-self="1">' + svg(ICONS.key) + '修改密码</button>';
      } else {
        actions =
          '<button class="btn btn-sm" data-uaction="role" data-id="' + u.id + '"' +
            ' data-role="' + (u.is_admin ? 'user' : 'admin') + '"' + guard + '>' +
            svg(ICONS.shield) + (u.is_admin ? '降为普通成员' : '设为管理员') + '</button>' +
          '<button class="btn btn-sm" data-uaction="password" data-id="' + u.id + '">' +
            svg(ICONS.key) + '重置密码</button>' +
          '<button class="btn btn-sm btn-danger" data-uaction="delete" data-id="' + u.id + '"' +
            ' data-name="' + esc(u.username) + '"' + guard + '>' +
            svg(ICONS.trash) + '删除</button>';
      }

      return '<div class="lib-card user-card rise">' +
          '<div class="user-avatar' + (u.is_self ? ' self' : '') + '">' +
            esc(String(u.username).slice(0, 1)) + '</div>' +
          '<div class="user-body">' +
            '<div class="user-name"><h2>' + esc(u.username) + '</h2>' + roleTag + '</div>' +
            '<div class="user-meta">' +
              '<span>创建于 ' + fmtTime(u.created_at) + '</span>' +
              '<span>ID <b>' + u.id + '</b></span>' +
            '</div>' +
          '</div>' +
          '<div class="user-actions">' + actions + '</div>' +
        '</div>';
    }).join('');

    $$('[data-uaction]', list).forEach(function (btn) {
      btn.addEventListener('click', function () {
        var action = this.getAttribute('data-uaction');
        var id = Number(this.getAttribute('data-id'));

        if (action === 'role') {
          var role = this.getAttribute('data-role');
          this.disabled = true;
          api('/api/users/' + id + '/role', { method: 'PUT', body: { role: role } })
            .then(function () { toast('角色已更新', 'ok'); return loadUsers(); })
            .catch(function (e) { toast(e.message, 'err'); loadUsers(); });
        } else if (action === 'password') {
          openResetPassword(id, this.getAttribute('data-self') === '1');
        } else if (action === 'delete') {
          confirmDeleteUser(id, this.getAttribute('data-name'));
        }
      });
    });
  }

  function passwordHint() {
    return '至少 8 位，且需包含字母、数字、符号中的至少两类。';
  }

  function openUserForm() {
    var modal = openModal(
      '<form id="user-form">' +
        '<div class="modal-head">' +
          '<h3>新建账号</h3>' +
          '<p>管理员可以管理书库与账号；普通成员只能浏览封面墙、搜索和下载。</p>' +
        '</div>' +
        '<div class="modal-body">' +
          '<div class="field">' +
            '<label for="u-name">用户名</label>' +
            '<input id="u-name" type="text" class="input" maxlength="32" required ' +
              'autocomplete="off" placeholder="字母、数字、下划线、点、短横线">' +
          '</div>' +
          '<div class="field">' +
            '<label for="u-pass">密码</label>' +
            '<input id="u-pass" type="password" class="input" required autocomplete="new-password">' +
            '<div class="hint">' + passwordHint() + '</div>' +
          '</div>' +
          '<div class="field">' +
            '<label for="u-role">角色</label>' +
            '<select id="u-role" class="select" style="width:100%">' +
              '<option value="user">普通成员 —— 只能浏览与下载</option>' +
              '<option value="admin">管理员 —— 可管理书库与账号</option>' +
            '</select>' +
          '</div>' +
          '<div id="u-error" class="alert error u-hidden"></div>' +
        '</div>' +
        '<div class="modal-foot">' +
          '<button type="button" data-close class="btn">取消</button>' +
          '<button type="submit" class="btn btn-primary">' + svg(ICONS.plus) + '创建账号</button>' +
        '</div>' +
      '</form>'
    );

    page.$('#user-form').addEventListener('submit', function (ev) {
      ev.preventDefault();
      var err = page.$('#u-error');

      function fail(msg) {
        err.innerHTML = svg(ICONS.alert) + '<span>' + esc(msg) + '</span>';
        err.classList.remove('u-hidden');
      }

      var username = page.$('#u-name').value.trim();
      var password = page.$('#u-pass').value;
      if (!username) { fail('请填写用户名'); return; }
      if (!password) { fail('请填写密码'); return; }
      err.classList.add('u-hidden');

      var btn = page.$('button[type="submit"]');
      btn.disabled = true;
      btn.textContent = '创建中…';

      api('/api/users', {
        method: 'POST',
        body: { username: username, password: password, role: page.$('#u-role').value }
      }).then(function () {
        modal.close();
        toast('账号已创建', 'ok');
        return loadUsers();
      }).catch(function (e) {
        fail(e.message);
        btn.disabled = false;
        btn.innerHTML = svg(ICONS.plus) + '创建账号';
      });
    });
  }

  function openResetPassword(id, isSelf) {
    var modal = openModal(
      '<form id="rp-form">' +
        '<div class="modal-head">' +
          '<h3>' + (isSelf ? '修改密码' : '重置密码') + '</h3>' +
          '<p>' + (isSelf
            ? '修改成功后，其他设备上的登录状态会立即失效。'
            : '重置后该账号在所有设备上的登录会立即失效，需要用新密码重新登录。') + '</p>' +
        '</div>' +
        '<div class="modal-body">' +
          '<div class="field">' +
            '<label for="rp-pass">新密码</label>' +
            '<input id="rp-pass" type="password" class="input" required autocomplete="new-password">' +
            '<div class="hint">' + passwordHint() + '</div>' +
          '</div>' +
          '<div class="field">' +
            '<label for="rp-pass2">确认新密码</label>' +
            '<input id="rp-pass2" type="password" class="input" required autocomplete="new-password">' +
          '</div>' +
          '<div id="rp-error" class="alert error u-hidden"></div>' +
        '</div>' +
        '<div class="modal-foot">' +
          '<button type="button" data-close class="btn">取消</button>' +
          '<button type="submit" class="btn btn-primary">' + svg(ICONS.key) +
            (isSelf ? '保存新密码' : '确认重置') + '</button>' +
        '</div>' +
      '</form>'
    );

    page.$('#rp-form').addEventListener('submit', function (ev) {
      ev.preventDefault();
      var err = page.$('#rp-error');

      function fail(msg) {
        err.innerHTML = svg(ICONS.alert) + '<span>' + esc(msg) + '</span>';
        err.classList.remove('u-hidden');
      }

      var p1 = page.$('#rp-pass').value;
      if (p1 !== page.$('#rp-pass2').value) { fail('两次输入的密码不一致'); return; }
      err.classList.add('u-hidden');

      var btn = page.$('button[type="submit"]');
      btn.disabled = true;
      btn.textContent = '提交中…';

      api('/api/users/' + id + '/password', { method: 'PUT', body: { password: p1 } })
        .then(function () {
          modal.close();
          if (isSelf) {
            toast('密码已更新，请重新登录', 'ok');
            setTimeout(function () { location.href = '/login'; }, 900);
            return;
          }
          toast('密码已重置，该账号的登录已全部失效', 'ok');
          return loadUsers();
        })
        .catch(function (e) {
          fail(e.message);
          btn.disabled = false;
          btn.innerHTML = svg(ICONS.key) + (isSelf ? '保存新密码' : '确认重置');
        });
    });
  }

  function confirmDeleteUser(id, name) {
    var modal = openModal(
      '<div class="modal-head">' +
        '<h3>删除账号</h3>' +
        '<p>将删除「<b>' + esc(name) + '</b>」及其全部登录会话。' +
          '该账号下载过的电子书不受影响。</p>' +
      '</div>' +
      '<div class="modal-foot">' +
        '<button type="button" data-close class="btn">取消</button>' +
        '<button id="confirm-udel" class="btn btn-primary danger-solid">' +
          svg(ICONS.trash) + '确认删除</button>' +
      '</div>'
    );

    var btn = page.$('#confirm-udel');
    btn.addEventListener('click', function () {
      btn.disabled = true;
      btn.textContent = '删除中…';
      api('/api/users/' + id, { method: 'DELETE' }).then(function () {
        modal.close();
        toast('账号已删除', 'ok');
        return loadUsers();
      }).catch(function (e) {
        btn.disabled = false;
        btn.innerHTML = svg(ICONS.trash) + '确认删除';
        toast(e.message, 'err');
      });
    });
  }

  /* ------------------------------------------------------------------ 目录选择器 */

  function openDirPicker(startPath, onPick) {
    var modal = openModal(
      '<div class="modal-head">' +
        '<h3>选择目录</h3>' +
        '<p>浏览磁盘并挑一个目录。只有可读目录才能作为书库。</p>' +
      '</div>' +
      '<div class="modal-body">' +
        '<div class="form-row">' +
          '<input id="picker-path" type="text" class="input mono" placeholder="输入或粘贴绝对路径" ' +
            'value="' + esc(startPath || '') + '">' +
          '<button id="picker-go" class="btn">前往</button>' +
        '</div>' +
        '<div id="picker-roots" class="roots" style="margin-top:11px"></div>' +
        '<div id="picker-list" class="picker-list" style="margin-top:11px"></div>' +
        '<div id="picker-error" class="alert error u-hidden" style="margin-top:11px"></div>' +
      '</div>' +
      '<div class="modal-foot split">' +
        '<span id="picker-current" class="current-path"></span>' +
        '<span style="display:flex;gap:9px">' +
          '<button type="button" data-close class="btn">取消</button>' +
          '<button id="picker-pick" class="btn btn-primary">' + svg(ICONS.check) + '选择此目录</button>' +
        '</span>' +
      '</div>', { wide: true }
    );

    var current = startPath || '';

    function render(data) {
      current = data.path;
      page.$('#picker-path').value = data.path;
      page.$('#picker-current').textContent = data.path;
      page.$('#picker-error').classList.add('u-hidden');

      var roots = page.$('#picker-roots');
      roots.innerHTML = (data.roots || []).map(function (r) {
        return '<button type="button" data-root="' + esc(r) + '" class="root-chip">' + esc(r) + '</button>';
      }).join('');
      $$('[data-root]', roots).forEach(function (b) {
        b.addEventListener('click', function () { load(this.getAttribute('data-root')); });
      });

      var list = page.$('#picker-list');
      var rows = '';
      if (data.parent) {
        rows += '<button type="button" data-dir="' + esc(data.parent) + '" class="picker-row">' +
          svg(ICONS.up, 'ico') + '<span class="nm">返回上级目录</span></button>';
      }
      if (data.dirs && data.dirs.length) {
        rows += data.dirs.map(function (d) {
          return '<button type="button" data-dir="' + esc(d.path) + '"' +
            ' class="picker-row' + (d.readable ? '' : ' unreadable') + '">' +
            svg(ICONS.folder, 'ico') +
            '<span class="nm">' + esc(d.name) + '</span>' +
            (d.readable ? '' : '<span style="font-size:10.5px;color:var(--danger)">无权限</span>') +
            svg(ICONS.chevron, 'chev') +
            '</button>';
        }).join('');
      } else {
        rows += '<div style="padding:26px;text-align:center;font-size:12.5px;color:var(--text-3)">' +
          '这个目录下没有子目录</div>';
      }

      list.innerHTML = rows;
      $$('[data-dir]', list).forEach(function (b) {
        b.addEventListener('click', function () { load(this.getAttribute('data-dir')); });
      });
    }

    function load(path) {
      var url = '/api/browse' + (path ? '?path=' + encodeURIComponent(path) : '');
      api(url).then(render).catch(function (e) {
        var box = page.$('#picker-error');
        box.innerHTML = svg(ICONS.alert) + '<span>' + esc(e.message) + '</span>';
        box.classList.remove('u-hidden');
      });
    }

    page.$('#picker-go').addEventListener('click', function () {
      load(page.$('#picker-path').value.trim());
    });
    page.$('#picker-path').addEventListener('keydown', function (ev) {
      if (ev.key === 'Enter') { ev.preventDefault(); load(this.value.trim()); }
    });
    page.$('#picker-pick').addEventListener('click', function () {
      if (!current) { toast('请先选择一个目录', 'err'); return; }
      onPick(current);
      modal.close();
    });

    load(startPath || '');
  }

  /* ------------------------------------------------------------------ 修改密码 */

  function openPasswordForm() {
    var modal = openModal(
      '<form id="pwd-form">' +
        '<div class="modal-head">' +
          '<h3>修改密码</h3>' +
          '<p>修改成功后，其他设备上的登录状态会立即失效。</p>' +
        '</div>' +
        '<div class="modal-body">' +
          '<div class="field"><label for="pwd-old">当前密码</label>' +
            '<input id="pwd-old" type="password" class="input" required autocomplete="current-password"></div>' +
          '<div class="field"><label for="pwd-new">新密码</label>' +
            '<input id="pwd-new" type="password" class="input" required minlength="8" autocomplete="new-password">' +
            '<div class="hint">' + passwordHint() + '</div></div>' +
          '<div class="field"><label for="pwd-new2">确认新密码</label>' +
            '<input id="pwd-new2" type="password" class="input" required minlength="8" autocomplete="new-password"></div>' +
          '<div id="pwd-error" class="alert error u-hidden"></div>' +
        '</div>' +
        '<div class="modal-foot">' +
          '<button type="button" data-close class="btn">取消</button>' +
          '<button type="submit" class="btn btn-primary">保存新密码</button>' +
        '</div>' +
      '</form>'
    );

    page.$('#pwd-form').addEventListener('submit', function (ev) {
      ev.preventDefault();
      var err = page.$('#pwd-error');

      function fail(msg) {
        err.innerHTML = svg(ICONS.alert) + '<span>' + esc(msg) + '</span>';
        err.classList.remove('u-hidden');
      }

      var oldPwd = page.$('#pwd-old').value;
      var newPwd = page.$('#pwd-new').value;
      if (newPwd !== page.$('#pwd-new2').value) { fail('两次输入的新密码不一致'); return; }
      err.classList.add('u-hidden');

      api('/api/auth/password', { method: 'POST', body: { old_password: oldPwd, new_password: newPwd } })
        .then(function () {
          modal.close();
          toast('密码已更新', 'ok');
        })
        .catch(function (e) { fail(e.message); });
    });
  }

  /* ------------------------------------------------------------------ 通用弹窗 */

  // 弹窗可能有嵌套（如新建书库里再开目录选择器），用计数保证滚动锁正确释放。
  var modalDepth = 0;

  function lockScroll(lock) {
    modalDepth += lock ? 1 : -1;
    if (modalDepth < 0) modalDepth = 0;
    document.body.style.overflow = modalDepth > 0 ? 'hidden' : '';
  }

  function openModal(inner, opts) {
    opts = opts || {};
    var wrap = document.createElement('div');
    wrap.className = 'modal';
    wrap.innerHTML =
      '<div class="modal-backdrop" data-close></div>' +
      '<div class="modal-panel' + (opts.wide ? ' wide' : '') +
        (opts.panelClass ? ' ' + opts.panelClass : '') + '" role="dialog" aria-modal="true">' +
        inner +
      '</div>';

    $('#modal-root').appendChild(wrap);
    lockScroll(true);

    var closed = false;
    function onKey(ev) { if (ev.key === 'Escape') close(); }
    function close() {
      if (closed) return;
      closed = true;
      document.removeEventListener('keydown', onKey);
      wrap.remove();
      lockScroll(false);
      // 弹窗自己可能也需要知道"我关了"（比如横幅面板要把首页横幅刷新一下）。
      // 在面板节点上派发一个不冒泡的事件，别的弹窗不会收到。
      var panel = wrap.querySelector('.modal-panel');
      if (panel) panel.dispatchEvent(new CustomEvent('modal-closed'));
    }
    document.addEventListener('keydown', onKey);

    $$('[data-close]', wrap).forEach(function (n) {
      n.addEventListener('click', function () { close(); });
    });

    return {
      root: wrap,
      close: close,
      $: function (sel) { return wrap.querySelector(sel); },
      $$: function (sel) { return $$(sel, wrap); }
    };
  }

  /* ------------------------------------------------------------------ 侧栏抽屉与顶栏滚动 */

  function openNav() {
    document.body.classList.add('nav-open');
    var btn = $('#nav-toggle');
    if (btn) btn.setAttribute('aria-label', '关闭导航');
  }

  function closeNav() {
    document.body.classList.remove('nav-open');
    var btn = $('#nav-toggle');
    if (btn) btn.setAttribute('aria-label', '打开导航');
  }

  function toggleNav() {
    if (document.body.classList.contains('nav-open')) closeNav(); else openNav();
  }

  /* ------------------------------------------------------------------ 路由 */

  function parseHash() {
    var hash = location.hash || '#/';
    hash = hash.replace(/^#/, '');
    var parts = hash.split('/').filter(function (s) { return s.length > 0; });

    if (parts.length === 0) return { name: 'home' };
    if (parts[0] === 'admin') {
      if (parts[1] === 'featured') return { name: 'featured' };
      return { name: 'admin' };
    }
    if (parts[0] === 'libraries') return { name: 'libraries' };
    if (parts[0] === 'users') return { name: 'users' };
    if (parts[0] === 'lib') return { name: 'wall', libraryId: Number(parts[1]) || 0 };
    if (parts[0] === 'book') return { name: 'detail', id: Number(parts[1]) || 0 };
    if (parts[0] === 'all') return { name: 'wall', libraryId: -1 };
    return { name: 'home' };
  }

  function route() {
    var r = parseHash();
    closeNav();

    // 每次换路由都回到页顶：首页可能已经滚到很下面，
    // 点"查看全部"进书库时不应把首页的滚动偏移一起带过去。
    window.scrollTo(0, 0);

    // 离开详情页后，任何在途的详情响应都不该再往 #view 里写。
    if (r.name !== 'detail') state.detailId = 0;
    // 首页横幅的定时器跟着首页视图走：renderHome 里会把还在走的那个停掉，
    // 只是这里不再强制停 —— 否则从首页点进横幅数据源、再返回首页时，
    // 轮播会莫名其妙地停住，得刷新页面才恢复。
    if (r.name !== 'home') state.featuredIdx = 0;

    if (r.name === 'featured') {
      if (!IS_ADMIN) { location.hash = '#/'; return; }
      state.libraryId = 0;
      renderSidebar();
      renderFeaturedSourceView();
      return;
    }
    if (r.name === 'admin') {
      if (!IS_ADMIN) { location.hash = '#/'; return; }
      state.libraryId = 0;
      renderSidebar();
      renderAdminView();
      return;
    }
    if (r.name === 'users') {
      state.libraryId = 0;
      renderSidebar();
      renderUsersView();
      return;
    }
    if (r.name === 'libraries') {
      state.libraryId = 0;
      renderSidebar();
      loadLibraries().then(renderLibrariesView);
      return;
    }
    if (r.name === 'detail') {
      renderSidebar();
      renderDetail(r.id);
      return;
    }
    if (r.name === 'home') {
      state.libraryId = 0;
      renderSidebar();
      renderHome();
      return;
    }

    // wall：全部书籍（-1）或某个书库
    state.libraryId = r.libraryId || 0;
    renderSidebar();
    renderWallShell();
    loadBooks(true);
  }

  /* ------------------------------------------------------------------ 初始化 */

  function init() {
    // 首屏入场动画的闸门：只在这一次渲染播放 .rise，渲染完 closeBootGate() 摘掉。
    document.documentElement.setAttribute(BOOT_ATTR, '1');
    setTimeout(closeBootGate, 1500); // 兜底：万一首屏渲染失败，也不要一直挂着

    paintCanvas();
    paintThemeButton();
    $('#theme-btn').addEventListener('click', toggleTheme);

    // 侧栏抽屉（窄屏）
    var navToggle = $('#nav-toggle');
    if (navToggle) navToggle.addEventListener('click', toggleNav);
    var scrim = $('#nav-scrim');
    if (scrim) scrim.addEventListener('click', closeNav);

    // 顶栏滚动阴影
    var topbar = $('#topbar');
    window.addEventListener('scroll', function () {
      if (!topbar) return;
      topbar.classList.toggle('is-scrolled', window.scrollY > 4);
    }, { passive: true });

    // 窗口变宽/变窄后，海报行与书库入口行是否还能翻页要重新算一遍
    window.addEventListener('resize', debounce(function () {
      state.libraries.slice(0, 6).forEach(function (lib) { updateRailNav(lib.id); });
      updateHomeLibNav();
    }, 150));

    var search = $('#search');
    search.addEventListener('input', debounce(function () {
      state.q = this.value.trim();
      var r = parseHash();
      if (r.name === 'wall') {
        loadBooks(true);
      } else {
        // 在首页/详情/管理页搜索时统一跳到"全部书籍"，免得横幅或侧栏干扰结果
        location.hash = '#/all';
      }
    }, 320));

    // "/" 聚焦搜索框（输入框里不抢）
    document.addEventListener('keydown', function (ev) {
      if (ev.key !== '/' || ev.metaKey || ev.ctrlKey || ev.altKey) return;
      var t = ev.target;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' ||
                t.tagName === 'SELECT' || t.isContentEditable)) return;
      ev.preventDefault();
      search.focus();
      search.select();
    });

    // 格式筛选（分段控件）
    $$('#format-seg button').forEach(function (btn) {
      btn.addEventListener('click', function () {
        state.format = this.getAttribute('data-format') || '';
        $$('#format-seg button').forEach(function (b) {
          b.setAttribute('aria-pressed', String(b === btn));
        });
        var r = parseHash();
        if (r.name === 'wall') loadBooks(true);
        else location.hash = state.format ? '#/all' : '#/';
      });
    });

    $('#sort').addEventListener('change', function () {
      state.sort = this.value;
      var r = parseHash();
      if (r.name === 'wall') loadBooks(true);
      else location.hash = '#/all';
    });

    // 用户菜单
    var userBtn = $('#user-btn');
    var userMenu = $('#user-menu');
    function closeMenu() {
      userMenu.classList.add('u-hidden');
      userBtn.setAttribute('aria-expanded', 'false');
    }
    userBtn.addEventListener('click', function (ev) {
      ev.stopPropagation();
      var open = userMenu.classList.toggle('u-hidden') === false;
      userBtn.setAttribute('aria-expanded', String(open));
    });
    document.addEventListener('click', closeMenu);
    userMenu.addEventListener('click', function (ev) { ev.stopPropagation(); });
    // 点菜单里的链接要收起菜单
    $$('a', userMenu).forEach(function (a) { a.addEventListener('click', closeMenu); });

    $('#btn-logout').addEventListener('click', function () {
      api('/api/auth/logout', { method: 'POST' }).then(function () {
        location.href = '/login';
      }).catch(function () { location.href = '/login'; });
    });

    $('#btn-password').addEventListener('click', function () {
      closeMenu();
      openPasswordForm();
    });

    // 卡片上的快捷下载：拦掉外层卡片链接的跳转
    function quickDownload(el) {
      var id = el.getAttribute('data-download');
      if (id) location.href = '/download/' + id;
    }
    document.addEventListener('click', function (ev) {
      var hit = ev.target.closest ? ev.target.closest('[data-download]') : null;
      if (!hit) return;
      ev.preventDefault();
      ev.stopPropagation();
      quickDownload(hit);
    });
    document.addEventListener('keydown', function (ev) {
      if (ev.key !== 'Enter' && ev.key !== ' ') return;
      var hit = ev.target.closest ? ev.target.closest('[data-download]') : null;
      if (!hit) return;
      ev.preventDefault();
      ev.stopPropagation();
      quickDownload(hit);
    });

    // 无限滚动：页脚进入视口就取下一页。IntersectionObserver 不可用时
    // 退回页脚上的"加载更多"按钮（renderFeedFoot 里已经渲染好）。
    if ('IntersectionObserver' in window) {
      var sentinel = document.createElement('div');
      sentinel.setAttribute('aria-hidden', 'true');
      document.body.appendChild(sentinel);
      state.sentinel = sentinel;
      new IntersectionObserver(function (entries) {
        entries.forEach(function (en) {
          if (!en.isIntersecting || state.loading) return;
          var foot = $('#feed-foot');
          if (!foot || foot.classList.contains('u-hidden')) return;
          if (!$('#more-btn')) return;
          loadBooks(false);
        });
      }, { rootMargin: '600px' }).observe(sentinel);
    }

    window.addEventListener('hashchange', route);

    // 首屏
    loadLibraries().then(route).catch(function (err) {
      toast(err.message, 'err');
    });

    watchDevReload();
  }

  /* ------------------------------------------------------------------ 开发模式
     只有服务端以 LIBRA_WEB_DIR 启动时，/__dev/reload 才存在（生产环境没有这个路由）。
     它返回一个随文件改动变化的时间戳：版本一变就整页刷新，于是改完 CSS / JS / 模板
     不用手动按 F5。探测一次失败就永久关闭，正常运行不会多出任何请求。 */
  function watchDevReload() {
    var version = null;
    var alive = true;

    function tick() {
      if (!alive) return;
      fetch('/__dev/reload', { credentials: 'same-origin', cache: 'no-store' })
        .then(function (r) { return r.ok ? r.json() : null; })
        .then(function (body) {
          if (!body || body.ok !== true) { alive = false; return; }
          if (version === null) {
            version = body.version;
            // 首次探测成功说明处于开发模式：在 <html> 上挂个标记，方便调试时确认
            document.documentElement.setAttribute('data-dev', '1');
            return;
          }
          if (body.version !== version) {
            version = body.version;
            location.reload();
          }
        })
        .catch(function () { alive = false; });
    }

    // 启动后 2 秒开始轮询：先让首屏把接口请求跑完
    setTimeout(function () {
      tick();
      var timer = setInterval(function () {
        if (!alive) { clearInterval(timer); return; }
        tick();
      }, 1000);
    }, 2000);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
