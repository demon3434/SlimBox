import { showAlert, showToast } from './dialog.js';
import { apiFetch } from '../api.js';

let isDragging = false;

export function isQueueDragging() {
  return isDragging;
}

export function bindQueueDragAndDrop(container, tasks, onReorderSuccess) {
  if (!container || !tasks || tasks.length <= 1) return;

  const cards = container.querySelectorAll('.film-sheet-card[data-task-id]');
  cards.forEach((card) => {
    const handle = card.querySelector('.task-drag-handle');
    if (!handle) return;

    // Enable dragging on handle mousedown
    handle.setAttribute('draggable', 'true');

    handle.addEventListener('dragstart', (e) => {
      isDragging = true;
      const taskId = card.dataset.taskId;
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', taskId);
      card.classList.add('is-dragging');
    });

    handle.addEventListener('dragend', () => {
      cleanupDragStyles(container);
      setTimeout(() => {
        isDragging = false;
      }, 150);
    });

    card.addEventListener('dragover', (e) => {
      if (!isDragging) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';

      const rect = card.getBoundingClientRect();
      const midY = rect.top + rect.height / 2;
      if (e.clientY < midY) {
        card.classList.add('drag-over-top');
        card.classList.remove('drag-over-bottom');
      } else {
        card.classList.add('drag-over-bottom');
        card.classList.remove('drag-over-top');
      }
    });

    card.addEventListener('dragleave', (e) => {
      // Remove indicator when leaving card boundary
      if (!card.contains(e.relatedTarget)) {
        card.classList.remove('drag-over-top', 'drag-over-bottom');
      }
    });

    card.addEventListener('drop', async (e) => {
      if (!isDragging) return;
      e.preventDefault();
      e.stopPropagation();

      const draggedId = e.dataTransfer.getData('text/plain');
      const targetId = card.dataset.taskId;
      if (!draggedId || draggedId === targetId) {
        cleanupDragStyles(container);
        isDragging = false;
        return;
      }

      const rect = card.getBoundingClientRect();
      const isBefore = e.clientY < (rect.top + rect.height / 2);

      const currentIds = tasks.map((t) => t.id);
      const draggedIndex = currentIds.indexOf(draggedId);
      if (draggedIndex === -1) return;

      currentIds.splice(draggedIndex, 1);
      const targetIndex = currentIds.indexOf(targetId);
      const insertIndex = isBefore ? targetIndex : targetIndex + 1;
      currentIds.splice(insertIndex, 0, draggedId);

      cleanupDragStyles(container);
      isDragging = false;

      // Persist to backend
      try {
        const res = await apiFetch('/api/v1/tasks/reorder', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ordered_ids: currentIds }),
        });
        if (res.ok) {
          showToast('队列顺序已更新', 'info', 1500);
          if (onReorderSuccess) onReorderSuccess();
        } else {
          showAlert({ title: '重排失败', message: '未能保存队列新顺序', type: 'error' });
        }
      } catch (err) {
        showAlert({ title: '网络错误', message: String(err), type: 'error' });
      }
    });
  });
}

function cleanupDragStyles(container) {
  if (!container) return;
  const allCards = container.querySelectorAll('.film-sheet-card');
  allCards.forEach((c) => {
    c.classList.remove('is-dragging', 'drag-over-top', 'drag-over-bottom');
  });
}
