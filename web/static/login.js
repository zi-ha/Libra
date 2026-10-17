(function () {
  'use strict';

  var ICON_ALERT = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" ' +
    'stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/>' +
    '<path d="M12 7.5v5.2M12 16.3v.2"/></svg>';

  var mode = 'login'; // login | register
  var allowRegister = false;

  var el = {
    tabLogin: document.getElementById('tab-login'),
    tabRegister: document.getElementById('tab-register'),
    form: document.getElementById('login-form'),
    username: document.getElementById('username'),
    password: document.getElementById('password'),
    password2: document.getElementById('password2'),
    pwdToggle: document.getElementById('pwd-toggle'),
    confirmWrap: document.getElementById('confirm-wrap'),
    error: document.getElementById('login-error'),
    submit: document.getElementById('submit-btn'),
    tip: document.getElementById('setup-tip'),
    pwdHint: document.getElementById('pwd-hint'),
    registerClosed: document.getElementById('register-closed')
  };

  var passwordVisible = false;

  function esc(s) {
    return String(s === null || s === undefined ? '' : s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function showError(msg) {
    el.error.innerHTML = ICON_ALERT + '<span>' + esc(msg) + '</span>';
    el.error.classList.remove('u-hidden');
  }

  function clearError() {
    el.error.classList.add('u-hidden');
    el.error.textContent = '';
  }

  function setPasswordVisible(next) {
    passwordVisible = !!next;
    var type = passwordVisible ? 'text' : 'password';
    el.password.type = type;
    el.password2.type = type;
    el.pwdToggle.textContent = passwordVisible ? '隐藏' : '显示';
    el.pwdToggle.setAttribute('aria-pressed', passwordVisible ? 'true' : 'false');
    el.pwdToggle.setAttribute('aria-label', passwordVisible ? '隐藏密码' : '显示密码');
  }

  function setMode(next) {
    mode = next;
    clearError();
    var login = mode === 'login';

    el.tabLogin.setAttribute('aria-selected', login ? 'true' : 'false');
    el.tabRegister.setAttribute('aria-selected', login ? 'false' : 'true');

    el.confirmWrap.classList.toggle('u-hidden', login);
    el.pwdHint.classList.toggle('u-hidden', login);
    el.password2.required = !login;
    el.password.autocomplete = login ? 'current-password' : 'new-password';
    el.submit.textContent = login ? '登录' : '创建账号';
    setPasswordVisible(false);
  }

  function applyMeta(meta) {
    allowRegister = !!meta.allow_register;
    el.tabRegister.classList.toggle('u-hidden', !allowRegister);
    el.registerClosed.classList.toggle('u-hidden', allowRegister || !!meta.needs_setup);

    if (meta.needs_setup) {
      el.tip.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" ' +
        'stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/>' +
        '<path d="M12 8.2v.2M12 11.4v4.6"/></svg>' +
        '<span>系统还没有任何账号。这里创建的<b>第一个账号将成为管理员</b>，请妥善保管密码。</span>';
      el.tip.classList.remove('u-hidden');
      setMode('register');
    } else {
      setMode('login');
    }
  }

  el.tabLogin.addEventListener('click', function () { setMode('login'); });
  el.tabRegister.addEventListener('click', function () { setMode('register'); });
  el.pwdToggle.addEventListener('click', function () { setPasswordVisible(!passwordVisible); });

  el.form.addEventListener('submit', function (ev) {
    ev.preventDefault();
    clearError();

    var username = el.username.value.trim();
    var password = el.password.value;

    if (!username) { showError('请输入用户名'); el.username.focus(); return; }
    // 注册时先做一次长度提示，真正的强度校验在服务端。
    if (mode === 'register' && password.length < 8) {
      showError('密码长度至少 8 位'); el.password.focus(); return;
    }
    if (mode === 'register' && password !== el.password2.value) {
      showError('两次输入的密码不一致');
      el.password2.focus();
      return;
    }

    el.submit.disabled = true;
    el.submit.textContent = mode === 'login' ? '登录中…' : '创建中…';

    var path = mode === 'login' ? '/api/auth/login' : '/api/auth/register';

    fetch(path, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: username, password: password })
    }).then(function (res) {
      return res.json().catch(function () { return { ok: false, error: '服务端返回异常 (' + res.status + ')' }; });
    }).then(function (body) {
      if (!body || body.ok === false) {
        throw new Error((body && body.error) || '操作失败');
      }
      location.href = '/';
    }).catch(function (err) {
      showError(err.message || '操作失败');
      el.submit.disabled = false;
      el.submit.textContent = mode === 'login' ? '登录' : '创建账号';
    });
  });

  // 探测系统状态：是否已初始化、是否开放注册
  fetch('/api/auth/me', { credentials: 'same-origin' })
    .then(function (res) { return res.json(); })
    .then(function (body) {
      if (body && body.ok && body.data) {
        if (body.data.authenticated) { location.href = '/'; return; }
        applyMeta(body.data);
      }
    })
    .catch(function () { setMode('login'); });
})();
