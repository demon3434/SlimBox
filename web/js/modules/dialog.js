// Custom Modern Dialog & Toast System for SlimBox
// Replaces browser-native alert() and confirm() with themed, glassmorphic UI components.

let activeDialogResolve = null;
let backdropEl = null;
let toastContainerEl = null;

function ensureBackdrop() {
  if (backdropEl) return backdropEl;

  backdropEl = document.createElement('div');
  backdropEl.className = 'custom-dialog-backdrop';
  backdropEl.id = 'customDialogBackdrop';
  backdropEl.innerHTML = `
    <div class="custom-dialog-box" id="customDialogBox" role="dialog" aria-modal="true">
      <div class="custom-dialog-header">
        <div class="custom-dialog-icon" id="customDialogIcon"></div>
        <div class="custom-dialog-title" id="customDialogTitle"></div>
        <button class="custom-dialog-close" id="customDialogClose" title="关闭 (Esc)">✕</button>
      </div>
      <div class="custom-dialog-body" id="customDialogBody"></div>
      <div class="custom-dialog-footer" id="customDialogFooter"></div>
    </div>
  `;

  document.body.appendChild(backdropEl);

  // Backdrop click dismisses (resolves false)
  backdropEl.addEventListener('click', (e) => {
    if (e.target === backdropEl) {
      closeDialog(false);
    }
  });

  const closeBtn = backdropEl.querySelector('#customDialogClose');
  if (closeBtn) {
    closeBtn.addEventListener('click', () => {
      closeDialog(false);
    });
  }

  // Global ESC key listener for dialog
  window.addEventListener('keydown', (e) => {
    if (!backdropEl.classList.contains('active')) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      closeDialog(false);
    } else if (e.key === 'Enter') {
      const activeEl = document.activeElement;
      // If focus is already on cancel button, let Enter trigger that button
      if (activeEl && activeEl.id === 'customDialogCancelBtn') {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      closeDialog(true);
    }
  });

  return backdropEl;
}

function ensureToastContainer() {
  if (toastContainerEl) return toastContainerEl;

  toastContainerEl = document.createElement('div');
  toastContainerEl.className = 'custom-toast-container';
  toastContainerEl.id = 'customToastContainer';
  document.body.appendChild(toastContainerEl);

  return toastContainerEl;
}

function closeDialog(result) {
  if (!backdropEl || !backdropEl.classList.contains('active')) return;
  backdropEl.classList.remove('active');
  const resolve = activeDialogResolve;
  activeDialogResolve = null;
  if (resolve) {
    resolve(result);
  }
}

/**
 * Show a sleek modal confirm dialog.
 * @param {Object} options
 * @param {string} options.title - Dialog title
 * @param {string} options.message - Dialog message body
 * @param {string} [options.confirmText='确定'] - Label for confirm button
 * @param {string} [options.cancelText='取消'] - Label for cancel button
 * @param {'danger'|'warning'|'primary'|'info'} [options.type='primary'] - Type/theme
 * @param {string} [options.icon] - Custom emoji or icon
 * @returns {Promise<boolean>}
 */
export function showConfirm({
  title,
  message,
  confirmText = '确定',
  cancelText = '取消',
  type = 'primary',
  icon = null,
} = {}) {
  const el = ensureBackdrop();

  if (!title) {
    title = type === 'danger' ? '确认操作' : '操作确认';
  }

  if (!icon) {
    if (type === 'danger') icon = '⚠️';
    else if (type === 'warning') icon = '⚡';
    else if (type === 'info') icon = 'ℹ️';
    else icon = '❓';
  }

  const iconEl = el.querySelector('#customDialogIcon');
  const titleEl = el.querySelector('#customDialogTitle');
  const bodyEl = el.querySelector('#customDialogBody');
  const footerEl = el.querySelector('#customDialogFooter');
  const closeBtn = el.querySelector('#customDialogClose');

  iconEl.className = `custom-dialog-icon ${type}`;
  iconEl.textContent = icon;
  titleEl.textContent = title;
  bodyEl.textContent = message;
  closeBtn.style.display = 'block';

  const confirmBtnClass = type === 'danger' ? 'btn btn-danger' : 'btn btn-primary';

  footerEl.innerHTML = `
    <button class="btn btn-secondary" id="customDialogCancelBtn">${escapeHtml(cancelText)}</button>
    <button class="${confirmBtnClass}" id="customDialogConfirmBtn">${escapeHtml(confirmText)}</button>
  `;

  const cancelBtn = footerEl.querySelector('#customDialogCancelBtn');
  const confirmBtn = footerEl.querySelector('#customDialogConfirmBtn');

  cancelBtn.addEventListener('click', () => closeDialog(false));
  confirmBtn.addEventListener('click', () => closeDialog(true));

  return new Promise((resolve) => {
    activeDialogResolve = resolve;
    el.classList.add('active');
    // Focus cancel button for danger, confirm button for standard
    if (type === 'danger') {
      cancelBtn.focus();
    } else {
      confirmBtn.focus();
    }
  });
}

/**
 * Show a sleek modal alert dialog.
 * @param {Object} options
 * @param {string} [options.title='系统提示'] - Dialog title
 * @param {string} options.message - Dialog message body
 * @param {string} [options.confirmText='我知道了'] - Confirm button text
 * @param {'info'|'warning'|'error'|'success'} [options.type='info'] - Alert type
 * @param {string} [options.icon] - Custom icon
 * @returns {Promise<void>}
 */
export function showAlert(options) {
  let title = '系统提示';
  let message = '';
  let confirmText = '我知道了';
  let type = 'info';
  let icon = null;

  if (typeof options === 'string') {
    message = options;
  } else if (options && typeof options === 'object') {
    title = options.title || title;
    message = options.message || '';
    confirmText = options.confirmText || confirmText;
    type = options.type || type;
    icon = options.icon || null;
  }

  const el = ensureBackdrop();

  if (!icon) {
    if (type === 'error') icon = '❌';
    else if (type === 'warning') icon = '⚠️';
    else if (type === 'success') icon = '✓';
    else icon = 'ℹ️';
  }

  const iconTypeClass = type === 'error' ? 'danger' : type;

  const iconEl = el.querySelector('#customDialogIcon');
  const titleEl = el.querySelector('#customDialogTitle');
  const bodyEl = el.querySelector('#customDialogBody');
  const footerEl = el.querySelector('#customDialogFooter');
  const closeBtn = el.querySelector('#customDialogClose');

  iconEl.className = `custom-dialog-icon ${iconTypeClass}`;
  iconEl.textContent = icon;
  titleEl.textContent = title;
  bodyEl.textContent = message;
  closeBtn.style.display = 'block';

  footerEl.innerHTML = `
    <button class="btn btn-primary" id="customDialogConfirmBtn" style="min-width: 100px;">${escapeHtml(confirmText)}</button>
  `;

  const confirmBtn = footerEl.querySelector('#customDialogConfirmBtn');
  confirmBtn.addEventListener('click', () => closeDialog(true));

  return new Promise((resolve) => {
    activeDialogResolve = resolve;
    el.classList.add('active');
    confirmBtn.focus();
  });
}

/**
 * Show a floating toast message.
 * @param {string} message - Content of the toast
 * @param {'info'|'success'|'warning'|'error'} [type='info'] - Style type
 * @param {number} [duration=3000] - Duration in ms
 */
export function showToast(message, type = 'info', duration = 3000) {
  const container = ensureToastContainer();

  const toast = document.createElement('div');
  toast.className = `custom-toast-item ${type}`;

  let icon = 'ℹ️';
  if (type === 'success') icon = '✓';
  else if (type === 'warning') icon = '⚠️';
  else if (type === 'error') icon = '❌';

  toast.innerHTML = `
    <span class="custom-toast-icon">${icon}</span>
    <span class="custom-toast-text">${escapeHtml(message)}</span>
  `;

  container.appendChild(toast);

  // Trigger animation next frame
  requestAnimationFrame(() => {
    toast.classList.add('active');
  });

  const removeTimer = setTimeout(() => {
    dismissToast(toast);
  }, duration);

  toast.addEventListener('click', () => {
    clearTimeout(removeTimer);
    dismissToast(toast);
  });
}

function dismissToast(toast) {
  toast.classList.remove('active');
  toast.classList.add('hide');
  setTimeout(() => {
    if (toast.parentNode) {
      toast.parentNode.removeChild(toast);
    }
  }, 260);
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

/**
 * Intercept native window.alert to route to custom modern showAlert
 */
export function initDialogSystem() {
  ensureBackdrop();
  ensureToastContainer();

  // Safely hook window.alert
  window.alert = function (msg) {
    return showAlert({
      title: '系统提示',
      message: String(msg),
      type: 'info',
    });
  };
}
