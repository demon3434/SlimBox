import { bus } from '../bus.js';
import { apiFetch } from '../api.js';

let statsTimer = null;
let popoverHideTimer = null;

function formatMem(gb) {
  if (gb === undefined || gb === null || isNaN(gb)) return '--';
  if (gb >= 1) {
    return `${gb.toFixed(1)}G`;
  }
  return `${Math.round(gb * 1024)}M`;
}

function formatUptime(seconds) {
  if (!seconds || seconds < 0) return '--';
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}天 ${h}小时 ${m}分`;
  if (h > 0) return `${h}小时 ${m}分`;
  return `${m}分钟`;
}

/**
 * Fetch and update hardware stats (CPU, memory, disk usage).
 */
export async function updateStats() {
  try {
    const res = await apiFetch('/api/v1/system/stats');
    if (!res.ok) return;
    const data = await res.json();


    // Top Header Pills
    const cpuEl = document.getElementById('statCpu');
    const memEl = document.getElementById('statMem');
    const diskEl = document.getElementById('statDisk');
    const barEl = document.getElementById('statDiskBar');

    const host = data.host || {};
    const container = data.container || {};
    const storage = data.storage || {};

    // 1. CPU Percentage
    if (cpuEl) {
      if (host.cpu_usage_pct !== undefined) {
        cpuEl.textContent = `${host.cpu_usage_pct.toFixed(0)}%`;
      } else {
        cpuEl.textContent = `${data.num_cpu || '--'}核`;
      }
    }

    // 2. 内存 (RAM) Percentage
    if (memEl) {
      if (host.mem_usage_pct !== undefined) {
        memEl.textContent = `${host.mem_usage_pct.toFixed(0)}%`;
      } else if (host.mem_used_gb !== undefined && host.mem_total_gb > 0) {
        memEl.textContent = `${((host.mem_used_gb / host.mem_total_gb) * 100).toFixed(0)}%`;
      } else {
        memEl.textContent = `${data.memory_alloc_mb ? data.memory_alloc_mb.toFixed(0) : '--'}M`;
      }
    }

    // 3. 硬盘 (Disk) Percentage
    const pctDisk = storage.usage_pct !== undefined ? storage.usage_pct : data.disk_usage_pct;
    if (diskEl && pctDisk !== undefined) {
      diskEl.textContent = `${pctDisk.toFixed(0)}%`;
    }

    // Populate Popover Dropdown Card
    renderPopover(data);

    // Notify listeners with complete stats (e.g. profiles module for hardware accelerators)
    bus.emit('system:stats', data);
  } catch (e) {
    console.warn('Failed to fetch system stats', e);
  }
}

function formatHumanBytes(gb) {
  if (gb === undefined || gb === null || isNaN(gb) || gb <= 0) return '0 MB';
  if (gb < 0.001) return '< 1 MB';
  if (gb < 1) {
    return `${Math.round(gb * 1024)} MB`;
  }
  return `${gb.toFixed(2)} GB`;
}

function renderPopover(data) {
  const host = data.host || {};
  const cont = data.container || {};
  const stor = data.storage || {};

  // 1. Host Machine Section
  const hostCpuEl = document.getElementById('popHostCpu');
  if (hostCpuEl) {
    const pct = host.cpu_usage_pct !== undefined ? host.cpu_usage_pct.toFixed(1) : '--';
    const cores = host.num_cpu || data.num_cpu || '--';
    hostCpuEl.textContent = `${pct}% (${cores} 核活跃)`;
  }

  const hostMemEl = document.getElementById('popHostMemCombined');
  if (hostMemEl && host.mem_total_gb !== undefined) {
    const used = host.mem_used_gb !== undefined ? `${host.mem_used_gb.toFixed(2)}G` : '--';
    const total = `${host.mem_total_gb.toFixed(2)}G`;
    const pct = host.mem_usage_pct !== undefined ? `${host.mem_usage_pct.toFixed(0)}%` : '--';
    hostMemEl.textContent = `${used} / ${total} (${pct})`;
  }

  const hostDiskEl = document.getElementById('popHostDiskCombined');
  if (hostDiskEl) {
    const used = (stor.used_gb !== undefined ? stor.used_gb : data.disk_used_gb) || 0;
    const total = (stor.total_gb !== undefined ? stor.total_gb : data.disk_total_gb) || 0;
    const pct = (stor.usage_pct !== undefined ? stor.usage_pct : data.disk_usage_pct) || 0;
    hostDiskEl.textContent = `${used.toFixed(1)}G / ${total.toFixed(1)}G (${pct.toFixed(0)}%)`;
  }

  const hostNetEl = document.getElementById('popHostNet');
  if (hostNetEl) {
    const down = host.net_rx_rate || '0 B/s';
    const up = host.net_tx_rate || '0 B/s';
    hostNetEl.innerHTML = `<span class="net-down">↓ ${down}</span><span class="net-sep">|</span><span class="net-up">↑ ${up}</span>`;
  }

  // 2. Container Environment Section
  const contModeEl = document.getElementById('popContMode');
  if (contModeEl) {
    if (cont.is_container) {
      contModeEl.textContent = cont.cgroup_version ? `Docker 容器 (${cont.cgroup_version})` : 'Docker 容器';
    } else {
      contModeEl.textContent = '原生宿主机进程';
    }
  }

  const contMemEl = document.getElementById('popContMemCombined');
  if (contMemEl) {
    if (cont.mem_limit_gb > 0) {
      const limitStr = formatHumanBytes(cont.mem_limit_gb);
      const usedStr = formatHumanBytes(cont.mem_used_gb);
      contMemEl.textContent = `限额 ${limitStr} (已用 ${usedStr})`;
    } else {
      const usedStr = cont.mem_used_gb > 0 ? ` (占用 ${formatHumanBytes(cont.mem_used_gb)})` : '';
      contMemEl.textContent = `无限制${usedStr}`;
    }
  }

  const storPathEl = document.getElementById('popStoragePath');
  if (storPathEl) {
    storPathEl.textContent = stor.path || data.storage_path || '/data';
  }

  // 3. Hardware Acceleration & Network Service Section
  const hwEl = document.getElementById('popHardwareAccel');
  if (hwEl) {
    const accels = data.hardware_accelerators || [];
    if (accels.length > 0) {
      const names = accels.map((a) => {
        if (a === 'nvenc') return 'NVIDIA NVENC ⚡';
        if (a === 'qsv') return 'Intel QuickSync ⚡';
        if (a === 'amf') return 'AMD AMF ⚡';
        if (a === 'videotoolbox') return 'Apple VideoToolbox ⚡';
        return a.toUpperCase();
      });
      hwEl.textContent = names.join(' / ');
      hwEl.style.color = '#34d399';
    } else {
      hwEl.textContent = 'CPU 软编 (通用保底)';
      hwEl.style.color = 'var(--text-secondary, #94a3b8)';
    }
  }

  const ipsEl = document.getElementById('popHostIPs');
  if (ipsEl) {
    const ips = data.host_ips || [];
    if (ips.length > 0) {
      const port = window.location.port ? `:${window.location.port}` : ':8080';
      ipsEl.textContent = ips.map((ip) => `${ip}${port}`).join(' | ');
    } else {
      ipsEl.textContent = '127.0.0.1:8080';
    }
  }
}

/**
 * Setup popover hover & mouse interactions with debounce.
 */
function setupPopoverInteractions() {
  const wrapper = document.getElementById('statsPanelWrapper');
  const popover = document.getElementById('statsPopover');
  if (!wrapper || !popover) return;

  const show = () => {
    if (popoverHideTimer) {
      clearTimeout(popoverHideTimer);
      popoverHideTimer = null;
    }
    popover.classList.add('visible');
    popover.setAttribute('aria-hidden', 'false');
  };

  const hide = () => {
    if (popoverHideTimer) clearTimeout(popoverHideTimer);
    popoverHideTimer = setTimeout(() => {
      popover.classList.remove('visible');
      wrapper.classList.remove('active');
      popover.setAttribute('aria-hidden', 'true');
    }, 220); // 220ms debounce buffer
  };

  wrapper.addEventListener('mouseenter', show);
  wrapper.addEventListener('mouseleave', hide);

  // Click toggle on summary card for touch/mobile
  const summaryCard = document.getElementById('statSummaryCard');
  if (summaryCard) {
    summaryCard.addEventListener('click', (e) => {
      e.stopPropagation();
      if (popover.classList.contains('visible')) {
        popover.classList.remove('visible');
        wrapper.classList.remove('active');
        popover.setAttribute('aria-hidden', 'true');
      } else {
        show();
        wrapper.classList.add('active');
      }
    });
  }

  document.addEventListener('click', (e) => {
    if (!wrapper.contains(e.target)) {
      popover.classList.remove('visible');
      wrapper.classList.remove('active');
      popover.setAttribute('aria-hidden', 'true');
    }
  });
}

/**
 * Initialize hardware stats polling and hook auth unlocked event.
 */
export function initStats() {
  setupPopoverInteractions();
  updateStats();
  if (statsTimer) clearInterval(statsTimer);
  statsTimer = setInterval(updateStats, 3000);

  bus.on('auth:unlocked', () => {
    updateStats();
  });
}
