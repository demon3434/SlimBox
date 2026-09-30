import { formatBytes, formatDateTime, escapeHtml } from '../utils.js';
import { apiFetch } from '../api.js';
import { showConfirm, showAlert, showToast } from './dialog.js';
import { buildQuadrantDrawerHtml } from './task_drawer.js';
import { isAbortedTaskSelected, toggleAbortedTask } from './task_batch.js';

let expandedAbortedTaskId = null;
let prevAbortedSnapshot = null;

const TRASH_ICON = `<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 6h18m-2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m-6 5v6m4-6v6"/></svg>`;
const PLAY_ICON = `<svg viewBox="0 0 24 24" width="13" height="13" fill="currentColor"><path d="M6 4l15 8-15 8V4z"/></svg>`;

export function resetAbortedSnapshot() {
  prevAbortedSnapshot = null;
}

export function renderAbortedList(tasks, onRefresh) {
  const container = document.getElementById('abortedList');
  const countEl = document.getElementById('abortedCount');
  if (countEl) countEl.textContent = tasks.length;
  if (!container) return;

  const snapshot = tasks.map((t) => `${t.id}:${t.status}:${isAbortedTaskSelected(t.id)}`).join('|');
  if (prevAbortedSnapshot !== null && prevAbortedSnapshot === snapshot) return;
  prevAbortedSnapshot = snapshot;

  container.innerHTML = '';

  if (tasks.length === 0) {
    container.innerHTML = `<div style="text-align:center; padding: 1.2rem; color: var(--text-dim); font-size: 0.8rem;">暂无已终止或失败任务</div>`;
    return;
  }

  tasks.forEach((t) => {
    const isExpanded = (expandedAbortedTaskId === t.id);
    const isChecked = isAbortedTaskSelected(t.id);
    const card = document.createElement('div');
    card.className = `film-sheet-card${isExpanded ? ' expanded' : ''}${isChecked ? ' is-selected' : ''}`;
    card.dataset.taskId = t.id;

    const addedTimeStr = formatDateTime(t.created_at);
    let statusBadge = `<span style="color: var(--accent-danger, #ef4444); font-weight: 500; font-size: 0.75rem;">■ 用户中止</span> <span style="color: var(--text-dim); font-size: 0.75rem;">(残片已清理)</span>`;
    if (t.status === 'failed') {
      const errBrief = t.error_msg ? (t.error_msg.length > 35 ? t.error_msg.slice(0, 35) + '...' : t.error_msg) : '未知错误';
      statusBadge = `<span style="color: var(--accent-danger, #ef4444); font-weight: 500; font-size: 0.75rem;">⚠ 转码失败</span> <span style="color: var(--text-dim); font-size: 0.75rem;" title="${escapeHtml(t.error_msg || '')}">(${escapeHtml(errBrief)})</span>`;
    }
    const actionButtons = `<button class="btn btn-sm btn-start-task btn-retry" title="重新放入队列再次转码 (插入队尾)">${PLAY_ICON}</button><button class="btn btn-sm btn-del" title="删除记录与暂存文件">${TRASH_ICON}</button>`;

    const p = t.params || {};
    const profile = (p.profile_name || '默认').toUpperCase();
    const vCodec = (p.video_codec || '').toUpperCase();
    const specLabel = `${profile}${vCodec ? ` · ${vCodec}` : ''}`;
    const metaDetails = `<span title="规格: ${escapeHtml(specLabel)}">规格: ${escapeHtml(specLabel)}</span><span>体积: ${formatBytes(t.source_file_size)}</span>`;

    card.innerHTML = `
      <div class="film-sheet-header">
        <div style="display: flex; align-items: center; gap: 0.35rem; flex-shrink: 0;">
          <label class="task-checkbox-wrap" title="选择"><input type="checkbox" class="task-checkbox aborted-item-chk" ${isChecked ? 'checked' : ''}></label>
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
      if (expandedAbortedTaskId === t.id) {
        expandedAbortedTaskId = null;
        card.classList.remove('expanded');
        toggleBtn.textContent = '详情 ▼';
      } else {
        if (expandedAbortedTaskId) {
          const prev = container.querySelector('.film-sheet-card.expanded');
          if (prev) {
            prev.classList.remove('expanded');
            const prevBtn = prev.querySelector('.btn-toggle-expand');
            if (prevBtn) prevBtn.textContent = '详情 ▼';
          }
        }
        expandedAbortedTaskId = t.id;
        card.classList.add('expanded');
        toggleBtn.textContent = '收起 ▲';
      }
    });

    const chk = card.querySelector('.aborted-item-chk');
    if (chk) {
      chk.addEventListener('click', (e) => e.stopPropagation());
      chk.addEventListener('change', (e) => {
        e.stopPropagation();
        toggleAbortedTask(t.id, e.target.checked);
        card.classList.toggle('is-selected', e.target.checked);
      });
    }

    const delBtn = card.querySelector('.btn-del');
    if (delBtn) {
      delBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        const confirmed = await showConfirm({
          title: '彻底删除任务',
          message: `确定彻底删除已终止任务【${t.source_file_name}】及其关联文件吗？此操作无法撤销。`,
          confirmText: '确认删除',
          cancelText: '取消',
          type: 'danger',
          icon: '🗑️',
        });
        if (confirmed) {
          if (expandedAbortedTaskId === t.id) expandedAbortedTaskId = null;
          prevAbortedSnapshot = null;
          try {
            const res = await apiFetch(`/api/v1/tasks/${t.id}`, { method: 'DELETE' });
            if (res.ok) {
              showToast(`已删除任务: ${t.source_file_name}`, 'info');
              if (onRefresh) onRefresh();
            } else {
              const err = await res.json().catch(() => ({}));
              showAlert({ title: '删除失败', message: err.message || '无法删除任务', type: 'error' });
            }
          } catch (err) {
            showAlert({ title: '网络错误', message: String(err), type: 'error' });
          }
        }
      });
    }

    const retryBtn = card.querySelector('.btn-retry');
    if (retryBtn) {
      retryBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        try {
          const res = await apiFetch(`/api/v1/tasks/${t.id}/retry`, { method: 'POST' });
          if (res.ok) {
            showToast(`任务【${t.source_file_name}】已重新入队 (排在队尾)`, 'success');
            prevAbortedSnapshot = null;
            if (onRefresh) onRefresh();
          } else {
            const err = await res.json().catch(() => ({}));
            showAlert({ title: '重新入队失败', message: err.message || '无法重试任务', type: 'error' });
          }
        } catch (err) {
          showAlert({ title: '网络错误', message: String(err), type: 'error' });
        }
      });
    }

    container.appendChild(card);
  });
}
