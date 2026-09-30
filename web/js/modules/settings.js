import { bus } from '../bus.js';
import { getAuthToken, setAuthToken, apiFetch } from '../api.js';
import { showUnlockModal, promptLogout } from './auth.js';
import { getCurrentTheme, renderThemeGrid, updateThemeUI } from './theme.js';
import { showConfirm, showAlert, showToast } from './dialog.js';
import { formatBytes } from '../utils.js';
import { initCleanerModule } from './cleaner.js';
import { initGPUSettings } from './settings_gpu.js';

export function initSettingsModal() {
  const modal = document.getElementById('settingsModal');
  const openBtn = document.getElementById('openSettingsBtn');
  const closeBtn = document.getElementById('closeSettingsBtn');
  const saveBtn = document.getElementById('saveSettingsBtn');

  // Navigation views
  const viewMain = document.getElementById('viewSettingsMain');
  const viewTheme = document.getElementById('viewSettingsTheme');
  const viewStorage = document.getElementById('viewSettingsStorage');
  const viewOrphans = document.getElementById('viewSettingsOrphans');
  const viewAuth = document.getElementById('viewSettingsAuth');
  const viewGPU = document.getElementById('viewSettingsGPU');

  const menuItemTheme = document.getElementById('menuItemTheme');
  const menuItemStorage = document.getElementById('menuItemStorage');
  const menuItemCleanOrphans = document.getElementById('menuItemCleanOrphans');
  const menuItemAuth = document.getElementById('menuItemAuth');
  const menuItemGPU = document.getElementById('menuItemGPU');
  const currentOrphanDesc = document.getElementById('currentOrphanDesc');
  const modalBackBtn = document.getElementById('modalBackBtn');
  const settingsModalTitle = document.getElementById('settingsModalTitle');

  const cleaner = initCleanerModule((summary) => {
    if (!currentOrphanDesc) return;
    if (summary && summary.total_count > 0) {
      currentOrphanDesc.textContent = `⚠️ 发现 ${summary.total_count} 项可清理 (可释放 ${formatBytes(summary.total_orphan_bytes)})`;
      currentOrphanDesc.style.color = 'var(--accent-danger)';
    } else {
      currentOrphanDesc.textContent = '✨ 存储环境良好，未发现残留文件';
      currentOrphanDesc.style.color = '';
    }
  });

  function switchView(viewName) {
    [viewMain, viewTheme, viewStorage, viewOrphans, viewAuth, viewGPU].forEach((v) => {
      if (v) v.classList.remove('active');
    });

    if (viewName === 'theme') {
      if (viewTheme) viewTheme.classList.add('active');
      if (modalBackBtn) modalBackBtn.style.display = 'inline-flex';
      if (settingsModalTitle) settingsModalTitle.textContent = '🎨 个性化主题配色';
      renderThemeGrid();
    } else if (viewName === 'gpu') {
      const gView = viewGPU || document.getElementById('viewSettingsGPU');
      if (gView) gView.classList.add('active');
      if (modalBackBtn) modalBackBtn.style.display = 'inline-flex';
      if (settingsModalTitle) settingsModalTitle.textContent = '⚡ 硬件加速与 GPU 调度';
    } else if (viewName === 'storage') {
      if (viewStorage) viewStorage.classList.add('active');
      if (modalBackBtn) modalBackBtn.style.display = 'inline-flex';
      if (settingsModalTitle) settingsModalTitle.textContent = '⚙️ 系统参数';
      loadStorageSettings();
    } else if (viewName === 'orphans') {
      if (viewOrphans) viewOrphans.classList.add('active');
      if (modalBackBtn) modalBackBtn.style.display = 'inline-flex';
      if (settingsModalTitle) settingsModalTitle.textContent = '🧹 存储维护与残留文件清理';
      cleaner.performScan();
    } else if (viewName === 'auth') {
      if (viewAuth) viewAuth.classList.add('active');
      if (modalBackBtn) modalBackBtn.style.display = 'inline-flex';
      if (settingsModalTitle) settingsModalTitle.textContent = '🔑 访问控制与 API-Key';
      loadAuthSettings();
    } else {
      if (viewMain) viewMain.classList.add('active');
      if (modalBackBtn) modalBackBtn.style.display = 'none';
      if (settingsModalTitle) settingsModalTitle.textContent = '⚙️ 系统设置';
      updateThemeUI(getCurrentTheme());
      loadAuthSettings();
    }
  }

  // Initialize GPU settings helper module
  initGPUSettings((targetView) => {
    if (modal) modal.style.display = 'flex';
    switchView(targetView || 'gpu');
  });

  if (openBtn) {
    openBtn.addEventListener('click', () => {
      switchView('main');
      if (modal) modal.style.display = 'flex';
    });
  }

  if (closeBtn) {
    closeBtn.addEventListener('click', () => {
      if (modal) modal.style.display = 'none';
    });
  }

  if (modal) {
    modal.addEventListener('click', (e) => {
      if (e.target === modal) {
        modal.style.display = 'none';
      }
    });
  }

  // Drill-down routing
  if (menuItemTheme) menuItemTheme.addEventListener('click', () => switchView('theme'));
  const targetGpuBtn = menuItemGPU || document.getElementById('menuItemGPU');
  if (targetGpuBtn) {
    targetGpuBtn.addEventListener('click', () => switchView('gpu'));
  }
  if (menuItemStorage) menuItemStorage.addEventListener('click', () => switchView('storage'));
  if (menuItemCleanOrphans) menuItemCleanOrphans.addEventListener('click', () => switchView('orphans'));
  if (menuItemAuth) menuItemAuth.addEventListener('click', () => switchView('auth'));
  if (modalBackBtn) modalBackBtn.addEventListener('click', () => switchView('main'));

  async function loadStorageSettings() {
    try {
      const res = await apiFetch('/api/v1/system/settings');
      if (res.ok) {
        const s = await res.json();
        const delSrc = document.getElementById('setDeleteSource');
        const retHrs = document.getElementById('setRetention');
        const chunkThresh = document.getElementById('setChunkThreshold');
        if (delSrc) delSrc.checked = !!s.delete_source_after_transcode;
        if (retHrs) retHrs.value = s.retention_hours || 0;
        if (chunkThresh) chunkThresh.value = s.chunk_threshold_mb || 200;
      }
    } catch (e) {
      console.warn('Failed to load storage settings', e);
    }
  }

  async function loadAuthSettings() {
    const badge = document.getElementById('authStatusBadge');
    const statusText = document.getElementById('authStatusText');
    const sessionStatus = document.getElementById('settingsSessionStatus');
    const sessionBox = document.getElementById('settingsMainSessionBox');
    const tokensList = document.getElementById('activeTokensList');

    let authEnabled = false;
    try {
      const res = await apiFetch('/api/v1/auth/status');
      if (res.ok) {
        const data = await res.json();
        authEnabled = !!data.auth_enabled;
        if (data.auth_enabled) {
          if (menuItemAuth) menuItemAuth.style.display = 'flex';
          if (badge) badge.className = 'auth-status-badge';
          if (statusText) statusText.textContent = '已开启安全鉴权保护 (双模 RBAC 模式)';
        } else {
          if (menuItemAuth) menuItemAuth.style.display = 'none';
          if (badge) badge.className = 'auth-status-badge locked';
          if (statusText) statusText.textContent = '开放访问模式 (局域网免密)';
        }
      }
    } catch (e) {
      console.warn('Failed to get auth status', e);
    }

    const currentToken = getAuthToken();
    if (sessionBox) {
      sessionBox.style.display = authEnabled && currentToken ? 'flex' : 'none';
    }
    if (sessionStatus) {
      if (currentToken) {
        const masked = currentToken.length > 8 ? currentToken.slice(0, 5) + '...' + currentToken.slice(-4) : '已登录';
        sessionStatus.textContent = `已登录 (${masked})`;
      } else {
        sessionStatus.textContent = '未登录';
      }
    }

    if (tokensList) {
      try {
        const res = await apiFetch('/api/v1/auth/tokens');
        if (res.ok) {
          const data = await res.json();
          renderTokensList(data.tokens || []);
        } else if (res.status === 403) {
          tokensList.innerHTML = `<p class="p-desc" style="color:var(--text-dim); padding: 8px;">无法获取密钥列表 (当前为受限终端凭据，需要管理员权限)</p>`;
        } else {
          tokensList.innerHTML = `<p class="p-desc" style="color:var(--text-dim); padding: 8px;">无法获取密钥列表 (需要管理员凭据)</p>`;
        }
      } catch (e) {
        tokensList.innerHTML = `<p class="p-desc" style="color:var(--text-dim); padding: 8px;">加载失败: ${e}</p>`;
      }
    }
  }

  function renderTokensList(tokens) {
    const list = document.getElementById('activeTokensList');
    if (!list) return;
    list.innerHTML = '';

    if (tokens.length === 0) {
      list.innerHTML = `<p class="p-desc" style="color:var(--text-dim); padding: 8px;">暂无活跃密钥。生成新密钥后将自动开启安全验证。</p>`;
      return;
    }

    tokens.forEach((tok) => {
      const item = document.createElement('div');
      item.className = 'token-item-card';

      const lastUsedStr = tok.last_used_at ? new Date(tok.last_used_at).toLocaleString() : '从未使用';
      const createdStr = tok.created_at ? new Date(tok.created_at).toLocaleDateString() : '';

      item.innerHTML = `
        <div class="token-item-meta">
          <span class="token-item-label">${tok.label} <small style="color:var(--accent-cyan); font-weight:normal;">(${tok.role})</small></span>
          <span class="token-item-used">创建于: ${createdStr} · 最后活跃: ${lastUsedStr}</span>
        </div>
        <button class="btn btn-sm btn-danger btn-revoke" title="撤回注销此密钥" data-id="${tok.id}">
          <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" style="display:inline-block; vertical-align:-2px; margin-right:3px;"><path d="M3 6h18m-2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m-6 5v6m4-6v6"/></svg>撤回
        </button>
      `;

      const delBtn = item.querySelector('.btn-revoke');
      if (delBtn) {
        delBtn.addEventListener('click', async () => {
          const confirmed = await showConfirm({
            title: '撤回访问密钥',
            message: `确定要撤回密钥 [${tok.label}] 吗？\n撤回后使用该密钥的客户端将立即无法访问本系统。`,
            confirmText: '确认撤回',
            cancelText: '取消',
            type: 'danger',
            icon: '🗑️',
          });
          if (confirmed) {
            try {
              const res = await apiFetch(`/api/v1/auth/tokens/${tok.id}`, { method: 'DELETE' });
              if (res.ok) {
                showToast(`密钥 [${tok.label}] 已撤回注销`, 'warning');
                loadAuthSettings();
              } else if (res.status === 403) {
                showAlert({ title: '权限不足', message: '仅管理员可撤回密钥', type: 'error' });
              } else {
                showAlert({ title: '操作失败', message: '撤回密钥失败，请检查操作权限', type: 'error' });
              }
            } catch (e) {
              showAlert({ title: '网络请求错误', message: '撤回请求失败: ' + e, type: 'error' });
            }
          }
        });
      }

      list.appendChild(item);
    });
  }

  // Token creation & modal binding
  const btnCreateToken = document.getElementById('btnCreateToken');
  const tokenLabelInput = document.getElementById('newTokenLabel');
  const tokenCreatedModal = document.getElementById('tokenCreatedModal');
  const createdTokenText = document.getElementById('createdTokenText');
  const btnCopyCreatedToken = document.getElementById('btnCopyCreatedToken');
  const btnAdoptCreatedToken = document.getElementById('btnAdoptCreatedToken');
  const closeTokenCreatedBtn = document.getElementById('closeTokenCreatedBtn');
  const copySuccessTip = document.getElementById('copySuccessTip');
  let lastCreatedToken = '';

  if (btnCreateToken) {
    btnCreateToken.addEventListener('click', async () => {
      const label = tokenLabelInput ? tokenLabelInput.value.trim() : '';
      if (!label) {
        showAlert({
          title: '信息不完整',
          message: '请输入新密钥用途名称 (如: 桌面端CLI / 自动化脚本)',
          type: 'warning',
          icon: '✏️',
        });
        return;
      }

      try {
        const res = await apiFetch('/api/v1/auth/tokens', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ label: label, role: 'user' }),
        });
        if (res.ok) {
          const data = await res.json();
          lastCreatedToken = data.token ? data.token.token : '';
          if (createdTokenText) createdTokenText.textContent = lastCreatedToken;
          if (tokenLabelInput) tokenLabelInput.value = '';
          if (copySuccessTip) copySuccessTip.style.display = 'none';
          if (tokenCreatedModal) tokenCreatedModal.style.display = 'flex';
          loadAuthSettings();
        } else if (res.status === 403) {
          showAlert({ title: '权限不足', message: '仅管理员有权签发新密钥', type: 'error' });
        } else {
          showAlert({ title: '操作失败', message: '生成密钥失败，请检查操作权限', type: 'error' });
        }
      } catch (e) {
        showAlert({ title: '网络请求错误', message: '生成密钥请求失败: ' + e, type: 'error' });
      }
    });
  }

  if (btnCopyCreatedToken) {
    btnCopyCreatedToken.addEventListener('click', async () => {
      if (!lastCreatedToken) return;
      try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(lastCreatedToken);
        } else {
          const ta = document.createElement('textarea');
          ta.value = lastCreatedToken;
          document.body.appendChild(ta);
          ta.select();
          document.execCommand('copy');
          document.body.removeChild(ta);
        }

        const copyIcon = `<svg class="copy-icon" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg>`;
        const checkIcon = `<svg class="copy-icon" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>`;
        btnCopyCreatedToken.innerHTML = checkIcon;
        btnCopyCreatedToken.classList.add('copied');
        showToast('API Key 已复制到剪贴板', 'success');

        setTimeout(() => {
          btnCopyCreatedToken.innerHTML = copyIcon;
          btnCopyCreatedToken.classList.remove('copied');
        }, 1800);
      } catch (err) {
        showToast('复制失败，请手动选取密钥文本', 'error');
      }
    });
  }

  if (btnAdoptCreatedToken) {
    btnAdoptCreatedToken.addEventListener('click', () => {
      if (lastCreatedToken) {
        setAuthToken(lastCreatedToken);
        if (tokenCreatedModal) tokenCreatedModal.style.display = 'none';
        loadAuthSettings();
      }
    });
  }

  if (closeTokenCreatedBtn) {
    closeTokenCreatedBtn.addEventListener('click', () => {
      if (tokenCreatedModal) tokenCreatedModal.style.display = 'none';
    });
  }

  const btnRefreshTokens = document.getElementById('btnRefreshTokens');
  if (btnRefreshTokens) {
    btnRefreshTokens.addEventListener('click', () => loadAuthSettings());
  }

  const btnClearToken = document.getElementById('btnClearToken');
  if (btnClearToken) {
    btnClearToken.addEventListener('click', () => {
      promptLogout();
    });
  }

  if (saveBtn) {
    saveBtn.addEventListener('click', async () => {
      const delSrc = document.getElementById('setDeleteSource');
      const retHrs = document.getElementById('setRetention');
      const chunkThresh = document.getElementById('setChunkThreshold');

      const payload = {
        delete_source_after_transcode: delSrc ? delSrc.checked : false,
        retention_hours: retHrs ? parseInt(retHrs.value) || 0 : 0,
        chunk_threshold_mb: chunkThresh ? parseInt(chunkThresh.value) || 200 : 200,
      };

      try {
        const res = await apiFetch('/api/v1/system/settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (res.ok) {
          showToast('系统参数配置已成功更新！', 'success');
          bus.emit('settings:updated', payload);
          switchView('main');
        } else if (res.status === 403) {
          showAlert({ title: '权限不足', message: '仅管理员有权修改系统底层参数配置', type: 'error' });
        } else {
          showAlert({ title: '保存失败', message: '保存设置失败，请检查操作权限', type: 'error' });
        }
      } catch (e) {
        showAlert({ title: '网络请求错误', message: '保存设置失败: ' + e, type: 'error' });
      }
    });

    const btnResetChunk = document.getElementById('btnResetChunkThreshold');
    if (btnResetChunk) {
      btnResetChunk.addEventListener('click', () => {
        const chunkThresh = document.getElementById('setChunkThreshold');
        if (chunkThresh) {
          chunkThresh.value = 200;
          showToast('分片门限已重置为默认值 (200MB)，点击下方保存即可生效', 'info', 3000);
        }
      });
    }
  }
}

