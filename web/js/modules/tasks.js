import { bus } from '../bus.js';
import {
  formatBytes, formatDateTime, escapeHtml,
} from '../utils.js';
import { showConfirm, showAlert, showToast } from './dialog.js';
import { getAuthToken, apiFetch } from '../api.js';
import { buildQuadrantDrawerHtml } from './task_drawer.js';
import {
  isQueueTaskSelected, isHistoryTaskSelected,
  toggleQueueTask, toggleHistoryTask,
  syncTasksForBatch, initBatchManager,
} from './task_batch.js';
import { isQueueDragging, bindQueueDragAndDrop } from './task_reorder.js';
import { renderActiveTask, abortActiveTask, deleteActiveTask } from './task_active.js';
import { renderAbortedList, resetAbortedSnapshot } from './task_aborted.js';

let activePollingTimer = null;
let expandedQueueTaskId = null;
let expandedHistoryTaskId = null;
let prevQueueSnapshot = null;
let prevHistorySnapshot = null;

const TRASH_ICON = `<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18m-2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m-6 5v6m4-6v6"/></svg>`;

const DOWNLOAD_ICON = `<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>`;

const LINK_ICON = `<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`;

const DRAG_HANDLE_SVG = `<svg viewBox="0 0 18 14" width="18" height="14" fill="currentColor"><circle cx="2.5" cy="2.5" r="1.3"/><circle cx="6.5" cy="2.5" r="1.3"/><circle cx="10.5" cy="2.5" r="1.3"/><circle cx="14.5" cy="2.5" r="1.3"/><circle cx="2.5" cy="7" r="1.3"/><circle cx="6.5" cy="7" r="1.3"/><circle cx="10.5" cy="7" r="1.3"/><circle cx="14.5" cy="7" r="1.3"/><circle cx="2.5" cy="11.5" r="1.3"/><circle cx="6.5" cy="11.5" r="1.3"/><circle cx="10.5" cy="11.5" r="1.3"/><circle cx="14.5" cy="11.5" r="1.3"/></svg>`;

const PLAY_ICON = `<svg viewBox="0 0 24 24" width="13" height="13" fill="currentColor"><path d="M6 4l15 8-15 8V4z"/></svg>`;

export async function pollTasks() {
  try {
    const res = await apiFetch('/api/v1/tasks');
    if (!res.ok) return;
    const data = await res.json();
    const tasks = data.tasks || [];
    const allIds = new Set(tasks.map((t) => t.id));
    if (expandedQueueTaskId && !allIds.has(expandedQueueTaskId)) expandedQueueTaskId = null;
    if (expandedHistoryTaskId && !allIds.has(expandedHistoryTaskId)) expandedHistoryTaskId = null;

    const active = tasks.find((t) => t.status === 'transcoding');
    renderActiveTask(active);

    const queued = tasks.filter((t) => t.status === 'queued' || t.status === 'paused');
    if (!isQueueDragging()) {
      renderQueueList(queued);
    }

    const completed = tasks.filter((t) => t.status === 'completed');
    renderHistoryList(completed);

    const aborted = tasks.filter((t) => t.status === 'aborted' || t.status === 'failed');
    renderAbortedList(aborted, () => pollTasks());

    syncTasksForBatch(queued, completed, aborted);
  } catch (e) {
    console.warn('Tasks poll failed', e);
  }
}


export async function pauseQueueTask(taskId, fileName) {
  try {
    const res = await apiFetch(`/api/v1/tasks/${taskId}/pause`, { method: 'POST' });
    if (res.ok) {
      showToast(`已暂停: ${fileName}`, 'info');
      prevQueueSnapshot = null;
      pollTasks();
    } else {
      const err = await res.json().catch(() => ({}));
      showAlert({ title: '暂停失败', message: err.message || '无法暂停任务', type: 'error' });
    }
  } catch (e) {
    showAlert({ title: '网络错误', message: String(e), type: 'error' });
  }
}

export async function resumeQueueTask(taskId, fileName) {
  try {
    const res = await apiFetch(`/api/v1/tasks/${taskId}/resume`, { method: 'POST' });
    if (res.ok) {
      showToast(`已恢复排队: ${fileName}`, 'success');
      prevQueueSnapshot = null;
      pollTasks();
    } else {
      const err = await res.json().catch(() => ({}));
      showAlert({ title: '恢复失败', message: err.message || '无法恢复任务', type: 'error' });
    }
  } catch (e) {
    showAlert({ title: '网络错误', message: String(e), type: 'error' });
  }
}

export async function retryTask(taskId, fileName) {
  try {
    const res = await apiFetch(`/api/v1/tasks/${taskId}/retry`, { method: 'POST' });
    if (res.ok) {
      showToast(`任务【${fileName}】已重新加入队列`, 'success');
      prevHistorySnapshot = null;
      prevQueueSnapshot = null;
      pollTasks();
    } else {
      const err = await res.json().catch(() => ({}));
      showAlert({ title: '重新运行失败', message: err.message || '无法重新入队', type: 'error' });
    }
  } catch (e) {
    showAlert({ title: '网络错误', message: String(e), type: 'error' });
  }
}

export async function adjustTaskPriority(taskId, direction) {
  try {
    const res = await apiFetch(`/api/v1/tasks/${taskId}/priority`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ direction }),
    });
    if (res.ok) {
      prevQueueSnapshot = null;
      pollTasks();
    } else {
      const err = await res.json().catch(() => ({}));
      showAlert({ title: '调整失败', message: err.message || '无法调整任务优先级', type: 'error' });
    }
  } catch (e) {
    showAlert({ title: '网络错误', message: String(e), type: 'error' });
  }
}

export function renderQueueList(tasks) {
  const container = document.getElementById('queueList');
  const countEl = document.getElementById('queueCount');
  if (countEl) countEl.textContent = tasks.length;
  if (!container) return;

  const snapshot = tasks.map((t) => `${t.id}:${t.status}:${t.priority || 0}:${isQueueTaskSelected(t.id)}`).join('|');
  if (prevQueueSnapshot !== null && prevQueueSnapshot === snapshot) return;
  prevQueueSnapshot = snapshot;

  container.innerHTML = '';
  if (tasks.length === 0) {
    container.innerHTML = `<div style="text-align:center; padding: 1.2rem; color: var(--text-dim); font-size: 0.8rem;">队列中暂无等待任务</div>`;
    return;
  }

  tasks.forEach((t) => {
    const isExpanded = (expandedQueueTaskId === t.id);
    const isChecked = isQueueTaskSelected(t.id);
    const card = document.createElement('div');
    card.className = `film-sheet-card${isExpanded ? ' expanded' : ''}${isChecked ? ' is-selected' : ''}`;
    card.dataset.taskId = t.id;

    const profile = t.params && t.params.profile_name ? t.params.profile_name.toUpperCase() : '未配置';
    const addedTimeStr = formatDateTime(t.created_at);
    const isPaused = t.status === 'paused';
    const statusBadge = isPaused
      ? `<span style="color: var(--accent-amber);">⏸ 已暂停</span>`
      : `<span style="color: var(--accent-cyan);">⏳ 排队等待中</span>`;

    const prioButtons = `<button class="btn btn-sm btn-ghost btn-prio btn-prio-top" title="置顶优先">⬆️</button><button class="btn btn-sm btn-ghost btn-prio btn-prio-up" title="上移">▲</button><button class="btn btn-sm btn-ghost btn-prio btn-prio-down" title="下移">▼</button>`;
    const actionButtons = isPaused
      ? `<button class="btn btn-sm btn-start-task btn-resume-task" title="恢复排队转码">${PLAY_ICON}</button>${prioButtons}<button class="btn btn-sm btn-del" title="从队列中移除此任务">${TRASH_ICON}</button>`
      : `<button class="btn btn-sm btn-ghost btn-pause-task" title="暂停任务">⏸</button>${prioButtons}<button class="btn btn-sm btn-del" title="从队列中移除此任务">${TRASH_ICON}</button>`;

    card.innerHTML = `
      <div class="film-sheet-header">
        <div style="display: flex; align-items: center; gap: 0.35rem; flex-shrink: 0;">
          <span class="task-drag-handle" title="按住拖拽调整优先级">${DRAG_HANDLE_SVG}</span>
          <label class="task-checkbox-wrap" title="选择"><input type="checkbox" class="task-checkbox queue-item-chk" ${isChecked ? 'checked' : ''}></label>
        </div>
        <div class="film-sheet-main-info">
          <div class="film-sheet-title-row"><span class="film-sheet-filename" title="${escapeHtml(t.source_file_name)}">${escapeHtml(t.source_file_name)}</span></div>
          <div class="film-sheet-meta-row">${statusBadge}<span title="档位: ${escapeHtml(profile)}">档位: ${escapeHtml(profile)}</span><span>体积: ${formatBytes(t.source_file_size)}</span><span title="添加于: ${escapeHtml(addedTimeStr)}">添加于: ${escapeHtml(addedTimeStr)}</span></div>
        </div>
        <div class="film-sheet-actions">${actionButtons}<button class="btn-toggle-expand" title="展开/收起批片参数">${isExpanded ? '收起 ▲' : '详情 ▼'}</button></div>
      </div>
      ${buildQuadrantDrawerHtml(t, true)}`;

    const header = card.querySelector('.film-sheet-header');
    const toggleBtn = card.querySelector('.btn-toggle-expand');
    header.addEventListener('click', (e) => {
      if (e.target.closest('button:not(.btn-toggle-expand), a, .task-drag-handle, .task-checkbox-wrap, input')) return;
      if (expandedQueueTaskId === t.id) {
        expandedQueueTaskId = null;
        card.classList.remove('expanded');
        toggleBtn.textContent = '详情 ▼';
      } else {
        if (expandedQueueTaskId) {
          const prev = container.querySelector('.film-sheet-card.expanded');
          if (prev) {
            prev.classList.remove('expanded');
            const prevBtn = prev.querySelector('.btn-toggle-expand');
            if (prevBtn) prevBtn.textContent = '详情 ▼';
          }
        }
        expandedQueueTaskId = t.id;
        card.classList.add('expanded');
        toggleBtn.textContent = '收起 ▲';
      }
    });

    const chk = card.querySelector('.queue-item-chk');
    if (chk) {
      chk.addEventListener('click', (e) => e.stopPropagation());
      chk.addEventListener('change', (e) => {
        e.stopPropagation();
        toggleQueueTask(t.id, e.target.checked);
        card.classList.toggle('is-selected', e.target.checked);
      });
    }

    const pauseBtn = card.querySelector('.btn-pause-task');
    if (pauseBtn) pauseBtn.addEventListener('click', async (e) => { e.stopPropagation(); await pauseQueueTask(t.id, t.source_file_name); });
    const resumeBtn = card.querySelector('.btn-resume-task');
    if (resumeBtn) resumeBtn.addEventListener('click', async (e) => { e.stopPropagation(); await resumeQueueTask(t.id, t.source_file_name); });
    const prioTop = card.querySelector('.btn-prio-top');
    if (prioTop) prioTop.addEventListener('click', async (e) => { e.stopPropagation(); await adjustTaskPriority(t.id, 'top'); });
    const prioUp = card.querySelector('.btn-prio-up');
    if (prioUp) prioUp.addEventListener('click', async (e) => { e.stopPropagation(); await adjustTaskPriority(t.id, 'up'); });
    const prioDown = card.querySelector('.btn-prio-down');
    if (prioDown) prioDown.addEventListener('click', async (e) => { e.stopPropagation(); await adjustTaskPriority(t.id, 'down'); });

    const delBtn = card.querySelector('.btn-del');
    if (delBtn) {
      delBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const confirmed = await showConfirm({
          title: '从队列移除任务',
          message: `确定移除任务【${t.source_file_name}】并清理未完成的残片吗？`,
          confirmText: '确认删除',
          cancelText: '取消',
          type: 'danger',
          icon: '🗑️',
        });
        if (confirmed) {
          if (expandedQueueTaskId === t.id) expandedQueueTaskId = null;
          prevQueueSnapshot = null;
          await apiFetch(`/api/v1/tasks/${t.id}`, { method: 'DELETE' });
          showToast(`已移除任务: ${t.source_file_name}`, 'info');
          pollTasks();
        }
      });
    }

    container.appendChild(card);
  });

  // Bind Drag & Drop Reordering
  bindQueueDragAndDrop(container, tasks, () => {
    prevQueueSnapshot = null;
    pollTasks();
  });
}

export function renderHistoryList(tasks) {
  const container = document.getElementById('historyList');
  const countEl = document.getElementById('historyCount');
  if (countEl) countEl.textContent = tasks.length;
  if (!container) return;

  const snapshot = tasks.map((t) => `${t.id}:${t.status}:${t.output_file_size || 0}:${t.download_count || 0}:${isHistoryTaskSelected(t.id)}`).join('|');
  if (prevHistorySnapshot !== null && prevHistorySnapshot === snapshot) return;
  prevHistorySnapshot = snapshot;

  container.innerHTML = '';

  if (tasks.length === 0) {
    container.innerHTML = `<div style="text-align:center; padding: 1.2rem; color: var(--text-dim); font-size: 0.8rem;">暂无历史完成任务</div>`;
    return;
  }

  tasks.forEach((t) => {
    const isExpanded = (expandedHistoryTaskId === t.id);
    const isChecked = isHistoryTaskSelected(t.id);
    const card = document.createElement('div');
    card.className = `film-sheet-card${isExpanded ? ' expanded' : ''}${isChecked ? ' is-selected' : ''}`;
    card.dataset.taskId = t.id;

    let statusBadge = '';
    let actionButtons = '';
    let metaDetails = '';
    const addedTimeStr = formatDateTime(t.created_at);

    if (t.status === 'completed') {
      const orig = t.source_file_size || 0, comp = t.output_file_size || 0;
      const savings = orig > 0 ? Math.round((1 - comp / orig) * 100) : 0;
      const savingsLabel = savings > 0 ? `${savings}%⬇` : (savings < 0 ? `+${Math.abs(savings)}%⬆` : '0%');
      const savingsTip = savings > 0
        ? `文件体积缩小了 ${savings}%（由 ${formatBytes(orig)} 降至 ${formatBytes(comp)}，节省 ${formatBytes(orig - comp)}）`
        : (savings < 0 ? `文件体积增大了 ${Math.abs(savings)}%` : '体积未变');
      const curTok = getAuthToken();
      const dlUrl = curTok ? `/api/v1/tasks/${t.id}/download?key=${encodeURIComponent(curTok)}` : `/api/v1/tasks/${t.id}/download`;
      actionButtons = `<a href="${dlUrl}" class="btn btn-sm btn-download" download title="下载压缩成品视频">${DOWNLOAD_ICON}</a><button class="btn btn-sm btn-ghost btn-copy-link" title="复制直链 (可粘贴至 Motrix / Aria2 等多线程下载工具)">${LINK_ICON}</button><button class="btn btn-sm btn-del" title="彻底删除此任务与关联视频文件">${TRASH_ICON}</button>`;

      const p = t.params || {};
      const profile = (p.profile_name || '默认').toUpperCase();
      let resText = p.target_resolution ? (p.target_resolution === 'original' ? '原画' : p.target_resolution.toUpperCase()) : (p.target_width && p.target_height ? `${p.target_width}×${p.target_height}` : profile);
      const codec = p.video_codec || '';
      const specLabel = (profile === 'CUSTOM' ? `CUSTOM · ${resText}` : profile) + (codec ? ` · ${codec}` : '');
      metaDetails = `<span title="规格: ${escapeHtml(specLabel)}">规格: ${escapeHtml(specLabel)}</span><span>产出: ${formatBytes(comp)}</span>`;
    } else {
      const isFailed = t.status === 'failed';
      statusBadge = isFailed
        ? `<span style="color: var(--accent-danger); font-size: 0.75rem; font-weight: 600;">转码失败 (残片已清理)</span>`
        : `<span style="color: var(--accent-danger, #ef4444); font-size: 0.75rem; font-weight: 500;">■ 用户中止</span> <span style="color: var(--text-dim); font-size: 0.75rem;">(残片已清理)</span>`;
      actionButtons = `<button class="btn btn-sm btn-retry" title="重新放入队列再次转码">${PLAY_ICON}</button><button class="btn btn-sm btn-del" title="删除记录与暂存文件">${TRASH_ICON}</button>`;
    }

    card.innerHTML = `
      <div class="film-sheet-header">
        <div style="display: flex; align-items: center; gap: 0.35rem; flex-shrink: 0;">
          <label class="task-checkbox-wrap" title="选择"><input type="checkbox" class="task-checkbox history-item-chk" ${isChecked ? 'checked' : ''}></label>
        </div>
        <div class="film-sheet-main-info">
          <div class="film-sheet-title-row"><span class="film-sheet-filename" title="${escapeHtml(t.source_file_name)}">${escapeHtml(t.source_file_name)}</span></div>
          <div class="film-sheet-meta-row">${statusBadge}${metaDetails}<span>添加于: ${escapeHtml(addedTimeStr)}</span></div>
        </div>
        <div class="film-sheet-actions">${actionButtons}<button class="btn-toggle-expand" title="展开/收起详细批片档案">${isExpanded ? '收起 ▲' : '详情 ▼'}</button></div>
      </div>
      ${buildQuadrantDrawerHtml(t, false)}`;

    const header = card.querySelector('.film-sheet-header');
    const toggleBtn = card.querySelector('.btn-toggle-expand');
    header.addEventListener('click', (e) => {
      if (e.target.closest('button:not(.btn-toggle-expand), a, .task-checkbox-wrap, input')) return;
      if (expandedHistoryTaskId === t.id) {
        expandedHistoryTaskId = null;
        card.classList.remove('expanded');
        toggleBtn.textContent = '详情 ▼';
      } else {
        if (expandedHistoryTaskId) {
          const prev = container.querySelector('.film-sheet-card.expanded');
          if (prev) {
            prev.classList.remove('expanded');
            const prevBtn = prev.querySelector('.btn-toggle-expand');
            if (prevBtn) prevBtn.textContent = '详情 ▼';
          }
        }
        expandedHistoryTaskId = t.id;
        card.classList.add('expanded');
        toggleBtn.textContent = '收起 ▲';
      }
    });

    const chk = card.querySelector('.history-item-chk');
    if (chk) {
      chk.addEventListener('click', (e) => e.stopPropagation());
      chk.addEventListener('change', (e) => {
        e.stopPropagation();
        toggleHistoryTask(t.id, e.target.checked);
        card.classList.toggle('is-selected', e.target.checked);
      });
    }

    const delBtn = card.querySelector('.btn-del');
    if (delBtn) {
      delBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const confirmed = await showConfirm({
          title: '彻底删除任务',
          message: `确定彻底删除任务【${t.source_file_name}】及其关联的视频文件吗？此操作无法撤销。`,
          confirmText: '确认删除',
          cancelText: '取消',
          type: 'danger',
          icon: '🗑️',
        });
        if (confirmed) {
          if (expandedHistoryTaskId === t.id) expandedHistoryTaskId = null;
          prevHistorySnapshot = null;
          await apiFetch(`/api/v1/tasks/${t.id}`, { method: 'DELETE' });
          showToast(`已删除任务: ${t.source_file_name}`, 'info');
          pollTasks();
        }
      });
    }

    const dlBtn = card.querySelector('.btn-download');
    if (dlBtn) {
      dlBtn.addEventListener('click', () => {
        const curTok = getAuthToken();
        dlBtn.href = curTok ? `/api/v1/tasks/${t.id}/download?key=${encodeURIComponent(curTok)}` : `/api/v1/tasks/${t.id}/download`;
      });
    }

    const copyBtn = card.querySelector('.btn-copy-link');
    if (copyBtn) {
      copyBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        try {
          const token = getAuthToken();
          let url = `${window.location.origin}/api/v1/tasks/${t.id}/download`;
          if (token) url += `?key=${encodeURIComponent(token)}`;

          if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(url);
          } else {
            const ta = document.createElement('textarea');
            ta.value = url;
            document.body.appendChild(ta);
            ta.select();
            document.execCommand('copy');
            document.body.removeChild(ta);
          }
          showToast('已复制下载直链，可粘贴至 Motrix / Aria2', 'success', 3500);
        } catch (err) {
          showToast('复制直链失败: ' + err.message, 'danger');
        }
      });
    }

    const retryBtn = card.querySelector('.btn-retry');
    if (retryBtn) {
      retryBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        await retryTask(t.id, t.source_file_name);
      });
    }

    container.appendChild(card);
  });
}

export function initTasksPolling() {
  initBatchManager(() => {
    prevQueueSnapshot = null;
    prevHistorySnapshot = null;
    resetAbortedSnapshot();
    pollTasks();
  });

  pollTasks();
  if (activePollingTimer) clearInterval(activePollingTimer);
  activePollingTimer = setInterval(pollTasks, 1500);

  const refreshBtn = document.getElementById('refreshQueueBtn');
  if (refreshBtn) {
    refreshBtn.addEventListener('click', () => {
      prevQueueSnapshot = null;
      prevHistorySnapshot = null;
      resetAbortedSnapshot();
      pollTasks();
    });
  }

  const abortBtn = document.getElementById('abortActiveBtn');
  if (abortBtn) abortBtn.addEventListener('click', () => abortActiveTask(() => pollTasks()));

  const delActiveBtn = document.getElementById('delActiveBtn');
  if (delActiveBtn) delActiveBtn.addEventListener('click', () => deleteActiveTask(() => pollTasks()));

  bus.on('task:created', () => {
    prevQueueSnapshot = null;
    resetAbortedSnapshot();
    pollTasks();
  });

  bus.on('auth:unlocked', () => {
    prevQueueSnapshot = null;
    prevHistorySnapshot = null;
    resetAbortedSnapshot();
    pollTasks();
  });
}
