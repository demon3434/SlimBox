import { showConfirm, showToast, showAlert } from './dialog.js';
import { apiFetch } from '../api.js';

// Selected ID sets
export const selectedQueueTaskIds = new Set();
export const selectedHistoryTaskIds = new Set();
export const selectedAbortedTaskIds = new Set();

// Batch Mode States (dinoroar sticker style: default hidden, activated on demand)
let isQueueBatchMode = false;
let isHistoryBatchMode = false;
let isAbortedBatchMode = false;
let refreshCallback = null;

export function initBatchManager(onRefreshNeeded) { refreshCallback = onRefreshNeeded; bindBatchEvents(); }
export function isQueueInBatchMode() { return isQueueBatchMode; }
export function isHistoryInBatchMode() { return isHistoryBatchMode; }
export function isAbortedInBatchMode() { return isAbortedBatchMode; }
export function isQueueTaskSelected(id) { return selectedQueueTaskIds.has(id); }
export function isHistoryTaskSelected(id) { return selectedHistoryTaskIds.has(id); }
export function isAbortedTaskSelected(id) { return selectedAbortedTaskIds.has(id); }

export function toggleQueueTask(id, checked) {
  if (checked) selectedQueueTaskIds.add(id); else selectedQueueTaskIds.delete(id);
  updateBatchUI();
}

export function toggleHistoryTask(id, checked) {
  if (checked) selectedHistoryTaskIds.add(id); else selectedHistoryTaskIds.delete(id);
  updateBatchUI();
}

export function toggleAbortedTask(id, checked) {
  if (checked) selectedAbortedTaskIds.add(id); else selectedAbortedTaskIds.delete(id);
  updateBatchUI();
}

export function clearQueueSelection() { selectedQueueTaskIds.clear(); updateBatchUI(); }
export function clearHistorySelection() { selectedHistoryTaskIds.clear(); updateBatchUI(); }
export function clearAbortedSelection() { selectedAbortedTaskIds.clear(); updateBatchUI(); }

let latestQueueTasks = [];
let latestHistoryTasks = [];
let latestAbortedTasks = [];

export function syncTasksForBatch(queueTasks, historyTasks, abortedTasks) {
  latestQueueTasks = queueTasks || [];
  latestHistoryTasks = historyTasks || [];
  latestAbortedTasks = abortedTasks || [];

  // Prune any deleted task IDs from selection sets
  const currentQIds = new Set(latestQueueTasks.map((t) => t.id));
  for (const id of selectedQueueTaskIds) {
    if (!currentQIds.has(id)) selectedQueueTaskIds.delete(id);
  }

  const currentHIds = new Set(latestHistoryTasks.map((t) => t.id));
  for (const id of selectedHistoryTaskIds) {
    if (!currentHIds.has(id)) selectedHistoryTaskIds.delete(id);
  }

  const currentAIds = new Set(latestAbortedTasks.map((t) => t.id));
  for (const id of selectedAbortedTaskIds) {
    if (!currentAIds.has(id)) selectedAbortedTaskIds.delete(id);
  }

  // Auto exit batch mode if tasks became empty
  if (latestQueueTasks.length === 0 && isQueueBatchMode) {
    isQueueBatchMode = false;
    selectedQueueTaskIds.clear();
  }
  if (latestHistoryTasks.length === 0 && isHistoryBatchMode) {
    isHistoryBatchMode = false;
    selectedHistoryTaskIds.clear();
  }
  if (latestAbortedTasks.length === 0 && isAbortedBatchMode) {
    isAbortedBatchMode = false;
    selectedAbortedTaskIds.clear();
  }

  updateBatchUI();
}

function updateBatchUI() {
  // 1. Queue Batch Bar & Header
  const qList = document.getElementById('queueList');
  const qBar = document.getElementById('queueBatchBar');
  const qCountEl = document.getElementById('queueSelectedCount');
  const qSelectAll = document.getElementById('queueSelectAllCheckbox');
  const qStartBtn = document.getElementById('queueBatchStartBtn');
  const toggleQBtn = document.getElementById('toggleQueueBatchBtn');

  if (qList) {
    qList.classList.toggle('batch-mode-active', isQueueBatchMode && latestQueueTasks.length > 0);
  }

  if (toggleQBtn) {
    if (isQueueBatchMode && latestQueueTasks.length > 0) {
      toggleQBtn.textContent = '✓ 退出批量';
      toggleQBtn.classList.add('btn-batch-active');
    } else {
      toggleQBtn.textContent = '📋 批量操作';
      toggleQBtn.classList.remove('btn-batch-active');
    }
    toggleQBtn.style.display = latestQueueTasks.length > 0 ? 'inline-flex' : 'none';
  }

  const qSelectedCount = selectedQueueTaskIds.size;
  if (qBar && qCountEl) {
    qBar.style.display = (isQueueBatchMode && latestQueueTasks.length > 0) ? 'inline-flex' : 'none';
    qCountEl.textContent = qSelectedCount;
  }

  if (qSelectAll) {
    qSelectAll.checked = latestQueueTasks.length > 0 && qSelectedCount === latestQueueTasks.length;
    qSelectAll.indeterminate = qSelectedCount > 0 && qSelectedCount < latestQueueTasks.length;
  }

  let startableCount = 0;
  let pauseableCount = 0;
  for (const t of latestQueueTasks) {
    if (selectedQueueTaskIds.has(t.id)) {
      if (t.status === 'pending' || t.status === 'paused') startableCount++;
      if (t.status === 'queued') pauseableCount++;
    }
  }
  if (qStartBtn) {
    qStartBtn.title = startableCount > 0 ? `批量启动/恢复 (${startableCount})` : '批量启动/恢复';
    qStartBtn.disabled = startableCount === 0;
  }
  const qPauseBtn = document.getElementById('queueBatchPauseBtn');
  if (qPauseBtn) {
    qPauseBtn.title = pauseableCount > 0 ? `批量暂停 (${pauseableCount})` : '批量暂停';
    qPauseBtn.disabled = pauseableCount === 0;
  }

  // 2. History Batch Bar & Header
  const hList = document.getElementById('historyList');
  const hBar = document.getElementById('historyBatchBar');
  const hCountEl = document.getElementById('historySelectedCount');
  const hSelectAll = document.getElementById('historySelectAllCheckbox');
  const toggleHBtn = document.getElementById('toggleHistoryBatchBtn');

  if (hList) {
    hList.classList.toggle('batch-mode-active', isHistoryBatchMode && latestHistoryTasks.length > 0);
  }

  if (toggleHBtn) {
    if (isHistoryBatchMode && latestHistoryTasks.length > 0) {
      toggleHBtn.textContent = '✓ 退出批量';
      toggleHBtn.classList.add('btn-batch-active');
    } else {
      toggleHBtn.textContent = '📋 批量操作';
      toggleHBtn.classList.remove('btn-batch-active');
    }
    toggleHBtn.style.display = latestHistoryTasks.length > 0 ? 'inline-flex' : 'none';
  }

  const hSelectedCount = selectedHistoryTaskIds.size;
  if (hBar && hCountEl) {
    hBar.style.display = (isHistoryBatchMode && latestHistoryTasks.length > 0) ? 'inline-flex' : 'none';
    hCountEl.textContent = hSelectedCount;
  }

  if (hSelectAll) {
    hSelectAll.checked = latestHistoryTasks.length > 0 && hSelectedCount === latestHistoryTasks.length;
    hSelectAll.indeterminate = hSelectedCount > 0 && hSelectedCount < latestHistoryTasks.length;
  }



  const quickClearBtn = document.getElementById('btnQuickClearHistory');
  if (quickClearBtn) {
    quickClearBtn.style.display = (latestHistoryTasks.length > 0 && !isHistoryBatchMode) ? 'inline-flex' : 'none';
  }

  // 3. Aborted Batch Bar & Header
  const aList = document.getElementById('abortedList');
  const aBar = document.getElementById('abortedBatchBar');
  const aCountEl = document.getElementById('abortedSelectedCount');
  const aSelectAll = document.getElementById('abortedSelectAllCheckbox');
  const toggleABtn = document.getElementById('toggleAbortedBatchBtn');

  if (aList) {
    aList.classList.toggle('batch-mode-active', isAbortedBatchMode && latestAbortedTasks.length > 0);
  }

  if (toggleABtn) {
    if (isAbortedBatchMode && latestAbortedTasks.length > 0) {
      toggleABtn.textContent = '✓ 退出批量';
      toggleABtn.classList.add('btn-batch-active');
    } else {
      toggleABtn.textContent = '📋 批量操作';
      toggleABtn.classList.remove('btn-batch-active');
    }
    toggleABtn.style.display = latestAbortedTasks.length > 0 ? 'inline-flex' : 'none';
  }

  const aSelectedCount = selectedAbortedTaskIds.size;
  if (aBar && aCountEl) {
    aBar.style.display = (isAbortedBatchMode && latestAbortedTasks.length > 0) ? 'inline-flex' : 'none';
    aCountEl.textContent = aSelectedCount;
  }

  if (aSelectAll) {
    aSelectAll.checked = latestAbortedTasks.length > 0 && aSelectedCount === latestAbortedTasks.length;
    aSelectAll.indeterminate = aSelectedCount > 0 && aSelectedCount < latestAbortedTasks.length;
  }

  const aBatchRetry = document.getElementById('abortedBatchRetryBtn');
  if (aBatchRetry) {
    aBatchRetry.title = aSelectedCount > 0 ? `批量恢复入队 (${aSelectedCount})` : '批量恢复入队';
    aBatchRetry.disabled = aSelectedCount === 0;
  }
  const aBatchDel = document.getElementById('abortedBatchDeleteBtn');
  if (aBatchDel) {
    aBatchDel.title = aSelectedCount > 0 ? `彻底删除已选记录 (${aSelectedCount})` : '彻底删除已选记录';
    aBatchDel.disabled = aSelectedCount === 0;
  }

  const quickClearAbortedBtn = document.getElementById('btnQuickClearAborted');
  if (quickClearAbortedBtn) {
    quickClearAbortedBtn.style.display = (latestAbortedTasks.length > 0 && !isAbortedBatchMode) ? 'inline-flex' : 'none';
  }
}

function bindBatchEvents() {
  // Toggle Queue Batch Mode
  const toggleQBtn = document.getElementById('toggleQueueBatchBtn');
  if (toggleQBtn) {
    toggleQBtn.addEventListener('click', () => {
      isQueueBatchMode = !isQueueBatchMode;
      if (!isQueueBatchMode) {
        selectedQueueTaskIds.clear();
      }
      updateBatchUI();
      if (refreshCallback) refreshCallback();
    });
  }

  // Queue Select All
  const qSelectAll = document.getElementById('queueSelectAllCheckbox');
  if (qSelectAll) {
    qSelectAll.addEventListener('change', (e) => {
      if (e.target.checked) {
        latestQueueTasks.forEach((t) => selectedQueueTaskIds.add(t.id));
      } else {
        selectedQueueTaskIds.clear();
      }
      updateBatchUI();
      if (refreshCallback) refreshCallback();
    });
  }

  // Queue Exit / Done
  const qCancel = document.getElementById('queueBatchCancelBtn');
  if (qCancel) {
    qCancel.addEventListener('click', () => {
      isQueueBatchMode = false;
      selectedQueueTaskIds.clear();
      updateBatchUI();
      if (refreshCallback) refreshCallback();
    });
  }

  // Queue Batch Start / Resume
  const qBatchStart = document.getElementById('queueBatchStartBtn');
  if (qBatchStart) {
    qBatchStart.addEventListener('click', async () => {
      const pausedIds = latestQueueTasks.filter((t) => selectedQueueTaskIds.has(t.id) && t.status === 'paused').map((t) => t.id);
      const pendingIds = latestQueueTasks.filter((t) => selectedQueueTaskIds.has(t.id) && t.status === 'pending').map((t) => t.id);
      if (pausedIds.length === 0 && pendingIds.length === 0) return;
      try {
        let total = 0;
        if (pausedIds.length > 0) {
          const res = await apiFetch('/api/v1/tasks/batch-resume', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ task_ids: pausedIds }) });
          if (res.ok) total += pausedIds.length;
        }
        if (pendingIds.length > 0) {
          const res = await apiFetch('/api/v1/tasks/batch-start', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ task_ids: pendingIds }) });
          if (res.ok) total += pendingIds.length;
        }
        if (total > 0) {
          showToast(`已成功恢复/启动 ${total} 个任务`, 'success');
          selectedQueueTaskIds.clear();
          if (refreshCallback) refreshCallback();
        }
      } catch (e) {
        showAlert({ title: '网络错误', message: String(e), type: 'error' });
      }
    });
  }

  // Queue Batch Pause
  const qBatchPause = document.getElementById('queueBatchPauseBtn');
  if (qBatchPause) {
    qBatchPause.addEventListener('click', async () => {
      const queuedIds = latestQueueTasks.filter((t) => selectedQueueTaskIds.has(t.id) && t.status === 'queued').map((t) => t.id);
      if (queuedIds.length === 0) return;
      try {
        const res = await apiFetch('/api/v1/tasks/batch-pause', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ task_ids: queuedIds }) });
        if (res.ok) {
          showToast(`已成功暂停 ${queuedIds.length} 个任务`, 'info');
          selectedQueueTaskIds.clear();
          if (refreshCallback) refreshCallback();
        } else {
          const err = await res.json().catch(() => ({}));
          showAlert({ title: '暂停失败', message: err.message || '批量暂停任务异常', type: 'error' });
        }
      } catch (e) {
        showAlert({ title: '网络错误', message: String(e), type: 'error' });
      }
    });
  }

  // Helper for batch deletes
  async function promptBatchDelete({ title, message, confirmText, ids, selectionSet, successMsg, icon = '🗑️' }) {
    if (!ids || ids.length === 0) return;
    const confirmed = await showConfirm({
      title, message, confirmText, cancelText: '取消', type: 'danger', icon,
    });
    if (!confirmed) return;
    try {
      const res = await apiFetch('/api/v1/tasks/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ task_ids: ids }),
      });
      if (res.ok) {
        showToast(successMsg, 'info');
        if (selectionSet) selectionSet.clear();
        if (refreshCallback) refreshCallback();
      } else {
        showAlert({ title: '删除失败', message: '未能完全清理部分任务', type: 'error' });
      }
    } catch (e) {
      showAlert({ title: '网络错误', message: String(e), type: 'error' });
    }
  }

  // Queue Batch Delete
  const qBatchDel = document.getElementById('queueBatchDeleteBtn');
  if (qBatchDel) {
    qBatchDel.addEventListener('click', () => {
      const ids = Array.from(selectedQueueTaskIds);
      promptBatchDelete({
        title: '批量移除任务',
        message: `确定从队列中移除已选中的 ${ids.length} 个任务并清理暂存文件吗？`,
        confirmText: `删除 (${ids.length})`,
        ids,
        selectionSet: selectedQueueTaskIds,
        successMsg: `已成功移除 ${ids.length} 个任务`,
      });
    });
  }

  // Toggle History Batch Mode
  const toggleHBtn = document.getElementById('toggleHistoryBatchBtn');
  if (toggleHBtn) {
    toggleHBtn.addEventListener('click', () => {
      isHistoryBatchMode = !isHistoryBatchMode;
      if (!isHistoryBatchMode) selectedHistoryTaskIds.clear();
      updateBatchUI();
      if (refreshCallback) refreshCallback();
    });
  }

  // History Select All
  const hSelectAll = document.getElementById('historySelectAllCheckbox');
  if (hSelectAll) {
    hSelectAll.addEventListener('change', (e) => {
      if (e.target.checked) latestHistoryTasks.forEach((t) => selectedHistoryTaskIds.add(t.id));
      else selectedHistoryTaskIds.clear();
      updateBatchUI();
      if (refreshCallback) refreshCallback();
    });
  }

  // History Quick Clear All
  const btnQuickClearHistory = document.getElementById('btnQuickClearHistory');
  if (btnQuickClearHistory) {
    btnQuickClearHistory.addEventListener('click', () => {
      if (latestHistoryTasks.length === 0) return;
      promptBatchDelete({
        title: '清空历史任务',
        message: `确定彻底清空全部 ${latestHistoryTasks.length} 个历史任务并删除对应视频文件吗？此操作不可恢复！`,
        confirmText: `清空全部 (${latestHistoryTasks.length})`,
        ids: latestHistoryTasks.map((t) => t.id),
        selectionSet: selectedHistoryTaskIds,
        successMsg: `已清空全部 ${latestHistoryTasks.length} 个历史任务`,
        icon: '🧹',
      });
    });
  }

  // History Batch Delete
  const hBatchDel = document.getElementById('historyBatchDeleteBtn');
  if (hBatchDel) {
    hBatchDel.addEventListener('click', () => {
      const ids = Array.from(selectedHistoryTaskIds);
      promptBatchDelete({
        title: '批量删除历史任务',
        message: `确定彻底删除已选中的 ${ids.length} 个历史任务及成品视频文件吗？此操作不可撤销。`,
        confirmText: `彻底删除 (${ids.length})`,
        ids,
        selectionSet: selectedHistoryTaskIds,
        successMsg: `已彻底删除 ${ids.length} 个历史任务`,
      });
    });
  }

  // Aborted Batch Mode Toggle
  const toggleABtn = document.getElementById('toggleAbortedBatchBtn');
  if (toggleABtn) {
    toggleABtn.addEventListener('click', () => {
      isAbortedBatchMode = !isAbortedBatchMode;
      if (!isAbortedBatchMode) selectedAbortedTaskIds.clear();
      updateBatchUI();
      if (refreshCallback) refreshCallback();
    });
  }

  // Aborted Select All
  const aSelectAll = document.getElementById('abortedSelectAllCheckbox');
  if (aSelectAll) {
    aSelectAll.addEventListener('change', (e) => {
      if (e.target.checked) latestAbortedTasks.forEach((t) => selectedAbortedTaskIds.add(t.id));
      else selectedAbortedTaskIds.clear();
      updateBatchUI();
      if (refreshCallback) refreshCallback();
    });
  }

  // Aborted Quick Clear All
  const btnQuickClearAborted = document.getElementById('btnQuickClearAborted');
  if (btnQuickClearAborted) {
    btnQuickClearAborted.addEventListener('click', () => {
      if (latestAbortedTasks.length === 0) return;
      promptBatchDelete({
        title: '清空已终止记录',
        message: `确定彻底清空全部 ${latestAbortedTasks.length} 个已终止任务并删除对应暂存文件吗？此操作不可恢复！`,
        confirmText: `清空全部 (${latestAbortedTasks.length})`,
        ids: latestAbortedTasks.map((t) => t.id),
        selectionSet: selectedAbortedTaskIds,
        successMsg: `已清空全部 ${latestAbortedTasks.length} 个已终止记录`,
        icon: '🧹',
      });
    });
  }

  // Aborted Batch Retry
  const aBatchRetry = document.getElementById('abortedBatchRetryBtn');
  if (aBatchRetry) {
    aBatchRetry.addEventListener('click', async () => {
      const ids = Array.from(selectedAbortedTaskIds);
      if (ids.length === 0) return;
      try {
        const res = await apiFetch('/api/v1/tasks/batch-retry', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ task_ids: ids }),
        });
        if (res.ok) {
          showToast(`已成功将 ${ids.length} 个任务恢复至等待队列 (排在队尾)`, 'success');
          selectedAbortedTaskIds.clear();
          if (refreshCallback) refreshCallback();
        } else {
          const err = await res.json().catch(() => ({}));
          showAlert({ title: '恢复失败', message: err.message || '批量恢复任务异常', type: 'error' });
        }
      } catch (e) {
        showAlert({ title: '网络错误', message: String(e), type: 'error' });
      }
    });
  }

  // Aborted Batch Delete
  const aBatchDel = document.getElementById('abortedBatchDeleteBtn');
  if (aBatchDel) {
    aBatchDel.addEventListener('click', () => {
      const ids = Array.from(selectedAbortedTaskIds);
      promptBatchDelete({
        title: '批量删除已终止任务',
        message: `确定彻底删除已选中的 ${ids.length} 个已终止任务及关联源文件吗？此操作不可撤销。`,
        confirmText: `彻底删除 (${ids.length})`,
        ids,
        selectionSet: selectedAbortedTaskIds,
        successMsg: `已彻底删除 ${ids.length} 个已终止任务`,
      });
    });
  }
}
