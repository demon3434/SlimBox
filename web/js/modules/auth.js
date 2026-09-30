import { bus } from '../bus.js';
import { getAuthToken, setAuthToken } from '../api.js';
import { showConfirm } from './dialog.js';

let isAuthEnabled = false;
let isSystemInitialized = true;

export async function promptLogout() {
  const confirmed = await showConfirm({
    title: '退出登录',
    message: '确定要退出当前的管理员登录状态吗？\n退出后需重新输入密码或 API Key 方可管理系统。',
    confirmText: '退出登录',
    cancelText: '取消',
    type: 'danger',
    icon: '🚪',
  });
  if (confirmed) {
    setAuthToken('');
    updateLogoutButtonVisibility();
    const settingsModal = document.getElementById('settingsModal');
    if (settingsModal) settingsModal.style.display = 'none';
    showUnlockModal();
    bus.emit('auth:logged_out');
  }
}

export function updateLogoutButtonVisibility() {
  const topBtn = document.getElementById('topNavLogoutBtn');
  const mainBox = document.getElementById('settingsMainSessionBox');
  const token = getAuthToken();
  const shouldShow = isAuthEnabled && !!token;

  if (topBtn) {
    topBtn.style.display = shouldShow ? 'inline-flex' : 'none';
  }
  if (mainBox) {
    mainBox.style.display = shouldShow ? 'flex' : 'none';
  }
}

export function showUnlockModal() {
  if (isAuthEnabled && !isSystemInitialized) {
    showSetupModal();
    return;
  }
  hideSetupModal();
  const modal = document.getElementById('unlockModal');
  if (!modal) return;
  if (modal.style.display === 'flex') return;

  const input = document.getElementById('unlockInput');
  const err = document.getElementById('unlockError');
  if (err) err.style.display = 'none';
  if (input) input.value = '';
  modal.style.display = 'flex';
}

export function hideUnlockModal() {
  const modal = document.getElementById('unlockModal');
  if (modal) modal.style.display = 'none';
}

export function showSetupModal() {
  hideUnlockModal();
  const modal = document.getElementById('setupModal');
  if (!modal) return;
  if (modal.style.display === 'flex') return;

  const pinInput = document.getElementById('setupPinInput');
  const passInput = document.getElementById('setupPasswordInput');
  const err = document.getElementById('setupError');
  if (err) err.style.display = 'none';
  if (pinInput) pinInput.value = '';
  if (passInput) passInput.value = '';
  modal.style.display = 'flex';
}

export function hideSetupModal() {
  const modal = document.getElementById('setupModal');
  if (modal) modal.style.display = 'none';
}

export async function initAuth() {
  const unlockInput = document.getElementById('unlockInput');
  const unlockBtn = document.getElementById('btnSubmitUnlock');
  const unlockError = document.getElementById('unlockError');

  const setupPinInput = document.getElementById('setupPinInput');
  const setupPasswordInput = document.getElementById('setupPasswordInput');
  const setupBtn = document.getElementById('btnSubmitSetup');
  const setupError = document.getElementById('setupError');

  // Listen to 401 interceptor trigger
  bus.on('auth:required', showUnlockModal);

  // Setup handler (PIN + Master password)
  const doSetup = async () => {
    const pin = setupPinInput ? setupPinInput.value.trim() : '';
    const pass = setupPasswordInput ? setupPasswordInput.value.trim() : '';

    if (!pin || pin.length !== 6) {
      if (setupError) {
        setupError.textContent = '请输入控制台输出的 6 位数字 PIN 码';
        setupError.style.display = 'block';
      }
      return;
    }
    if (!pass || pass.length < 6) {
      if (setupError) {
        setupError.textContent = '管理员主密码长度不能少于 6 位';
        setupError.style.display = 'block';
      }
      return;
    }

    try {
      const res = await fetch('/api/v1/auth/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ pin, password: pass }),
      });
      const data = await res.json();
      if (res.ok && data.token) {
        isSystemInitialized = true;
        setAuthToken(data.token);
        hideSetupModal();
        bus.emit('auth:unlocked', data.token);
      } else {
        if (setupError) {
          setupError.textContent = data.message || '初始化失败，请核对 PIN 码';
          setupError.style.display = 'block';
        }
      }
    } catch (e) {
      if (setupError) {
        setupError.textContent = '初始化请求异常: ' + e;
        setupError.style.display = 'block';
      }
    }
  };

  if (setupBtn) setupBtn.addEventListener('click', doSetup);
  if (setupPasswordInput) {
    setupPasswordInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') doSetup();
    });
  }

  // Unlock handler (login with password or verify token)
  const doUnlock = async () => {
    const val = unlockInput ? unlockInput.value.trim() : '';
    if (!val) {
      if (unlockError) {
        unlockError.textContent = '请输入管理员主密码或访问密钥';
        unlockError.style.display = 'block';
      }
      return;
    }

    try {
      // If it has sb_ prefix, try verifying as token directly
      if (val.startsWith('sb_')) {
        const res = await fetch('/api/v1/auth/verify', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token: val }),
        });
        const data = await res.json();
        if (data.valid) {
          setAuthToken(val);
          hideUnlockModal();
          bus.emit('auth:unlocked', val);
          return;
        }
      }

      // Otherwise attempt admin master password login
      const loginRes = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: val }),
      });
      const loginData = await loginRes.json();
      if (loginRes.ok && loginData.token) {
        setAuthToken(loginData.token);
        hideUnlockModal();
        bus.emit('auth:unlocked', loginData.token);
      } else {
        if (unlockError) {
          unlockError.textContent = loginData.message || '管理员密码错误，请重新输入';
          unlockError.style.display = 'block';
        }
      }
    } catch (e) {
      if (unlockError) {
        unlockError.textContent = '验证请求失败: ' + e;
        unlockError.style.display = 'block';
      }
    }
  };

  if (unlockBtn) unlockBtn.addEventListener('click', doUnlock);
  if (unlockInput) {
    unlockInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') doUnlock();
    });
  }

  const topNavLogout = document.getElementById('topNavLogoutBtn');
  if (topNavLogout) {
    topNavLogout.addEventListener('click', promptLogout);
  }

  bus.on('auth:unlocked', () => {
    updateLogoutButtonVisibility();
  });

  // Initial status check
  try {
    const res = await fetch('/api/v1/auth/status');
    if (res.ok) {
      const status = await res.json();
      isAuthEnabled = !!status.auth_enabled;
      isSystemInitialized = !!status.initialized;

      if (!status.auth_enabled) {
        // Open local mode: no prompt, running freely
        updateLogoutButtonVisibility();
        return;
      }

      if (!status.initialized) {
        // Strict mode active but no master password set yet
        updateLogoutButtonVisibility();
        showSetupModal();
        return;
      }

      // Strict mode and initialized: check local token
      const saved = getAuthToken();
      if (!saved) {
        updateLogoutButtonVisibility();
        showUnlockModal();
      } else {
        const vRes = await fetch('/api/v1/auth/verify', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token: saved }),
        });
        const vData = await vRes.json();
        if (!vData.valid) {
          setAuthToken('');
          updateLogoutButtonVisibility();
          showUnlockModal();
        } else {
          updateLogoutButtonVisibility();
        }
      }
    }
  } catch (e) {
    console.warn('Failed to check auth status', e);
  }
}
