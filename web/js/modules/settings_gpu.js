import { apiFetch } from '../api.js';
import { showToast } from './dialog.js';
import { bus } from '../bus.js';

let cachedGPUConfig = null;

export async function fetchGPUConfig() {
  try {
    const res = await apiFetch('/api/v1/system/gpu-config');
    if (res.ok) {
      cachedGPUConfig = await res.json();
      return cachedGPUConfig;
    }
  } catch (err) {
    console.warn('[Settings GPU] Failed to fetch GPU config:', err);
  }
  return null;
}

export function initGPUSettings(onSwitchView) {
  const select = document.getElementById('selectPreferredGPU');
  const listEl = document.getElementById('gpuDeviceList');
  const refreshBtn = document.getElementById('btnRefreshGPUs');
  const saveBtn = document.getElementById('saveGPUSettingsBtn');
  const descEl = document.getElementById('currentGPUDesc');
  const badgeEl = document.getElementById('workbenchGPUBadge');
  const badgeName = document.getElementById('workbenchGPUName');

  async function refreshUI() {
    const cfg = await fetchGPUConfig();
    if (!cfg) return;

    if (select) {
      select.innerHTML = '';

      const autoOpt = document.createElement('option');
      autoOpt.value = 'auto';
      autoOpt.textContent = '⚡ 自动模式 (优先使用高性能独立显卡/核显)';
      select.appendChild(autoOpt);

      if (cfg.devices && cfg.devices.length > 0) {
        cfg.devices.forEach((dev) => {
          const opt = document.createElement('option');
          opt.value = dev.id;
          const status = dev.operational ? '可用' : '不可用/无驱动';
          opt.textContent = `[${dev.vendor.toUpperCase()}] ${dev.name} (${status})`;
          select.appendChild(opt);
        });
      }

      const cpuOpt = document.createElement('option');
      cpuOpt.value = 'cpu_only';
      cpuOpt.textContent = '⚪ 强制禁用硬件加速 (纯 CPU 软解软编，画质优先)';
      select.appendChild(cpuOpt);

      select.value = cfg.preferred_gpu || 'auto';
    }

    if (listEl) {
      if (!cfg.devices || cfg.devices.length === 0) {
        listEl.innerHTML = '<div style="color: var(--text-muted);">未检测到硬件加速设备，将默认采用 CPU 软解软编。</div>';
      } else {
        listEl.innerHTML = cfg.devices
          .map((dev) => {
            const tagColor = dev.operational ? 'var(--accent-success, #10b981)' : 'var(--accent-danger, #ef4444)';
            const statusText = dev.operational ? '● 探针正常' : '○ 驱动不可用';
            const isSel = dev.is_selected ? ' (当前激活)' : '';
            return `<div style="display: flex; justify-content: space-between; margin-bottom: 0.25rem;">
              <span><strong>${dev.name}</strong> <small style="color: var(--text-muted);">${dev.id}</small></span>
              <span style="color: ${tagColor}; font-weight: 500;">${statusText}${isSel}</span>
            </div>`;
          })
          .join('');
      }
    }

    // Update settings menu subtitle and workbench badge
    let label = '自动优选';
    if (cfg.preferred_gpu === 'cpu_only') {
      label = '纯 CPU 软编';
    } else if (cfg.active_gpu) {
      label = cfg.active_gpu.name.split(' ')[0] + ' ' + (cfg.active_gpu.type ? cfg.active_gpu.type.toUpperCase() : '');
    }
    if (descEl) descEl.textContent = `当前：${label}`;
    if (badgeName) badgeName.textContent = label;

    bus.emit('gpu:updated', cfg);
  }

  if (saveBtn) {
    saveBtn.addEventListener('click', async () => {
      if (!select) return;
      saveBtn.disabled = true;
      saveBtn.textContent = '保存中...';
      try {
        const res = await apiFetch('/api/v1/system/gpu-config', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ preferred_gpu: select.value })
        });
        if (res.ok) {
          showToast('GPU 算力配置已更新并实时生效', 'success');
          await refreshUI();
        } else {
          const err = await res.json();
          showToast(err.error || '保存 GPU 配置失败', 'error');
        }
      } catch (e) {
        showToast('网络请求异常: ' + e.message, 'error');
      } finally {
        saveBtn.disabled = false;
        saveBtn.textContent = '💾 保存显卡配置';
      }
    });
  }

  if (refreshBtn) {
    refreshBtn.addEventListener('click', async () => {
      refreshBtn.disabled = true;
      refreshBtn.textContent = '正在探测...';
      showToast('正在重新探测系统显卡硬件设备...', 'info');
      await refreshUI();
      refreshBtn.disabled = false;
      refreshBtn.textContent = '🔄 刷新设备';
    });
  }

  if (badgeEl && onSwitchView) {
    badgeEl.addEventListener('click', () => {
      onSwitchView('gpu');
    });
  }

  // Initial populate
  refreshUI();
  bus.on('auth:unlocked', () => refreshUI());
}
