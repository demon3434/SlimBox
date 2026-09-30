import { bus } from '../bus.js';
import { apiFetch } from '../api.js';
import { showConfirm, showAlert, showToast } from './dialog.js';
import { formatBytes } from '../utils.js';

let currentOrphanSummary = null;

export function initCleanerModule(onSummaryUpdate) {
  const btnScan = document.getElementById('btnScanOrphans');
  const btnClean = document.getElementById('btnCleanOrphans');
  const scanSpinner = document.getElementById('scanOrphansSpinner');
  const scanBtnText = document.getElementById('scanOrphansBtnText');
  const resultBox = document.getElementById('orphanScanResultBox');
  const summaryText = document.getElementById('orphanScanSummaryText');
  const reclaimableSize = document.getElementById('orphanReclaimableSize');
  const categoryGrid = document.getElementById('orphanCategoryGrid');

  async function performScan() {
    if (!btnScan) return;
    btnScan.disabled = true;
    if (scanSpinner) scanSpinner.style.display = 'inline-block';
    if (scanBtnText) scanBtnText.textContent = '扫描中...';

    try {
      const res = await apiFetch('/api/v1/system/orphans/scan');
      if (res.ok) {
        const summary = await res.json();
        currentOrphanSummary = summary;
        renderResult(summary);
        if (typeof onSummaryUpdate === 'function') {
          onSummaryUpdate(summary);
        }
      } else {
        showAlert({ title: '扫描失败', message: '无法获取残留文件扫描结果', type: 'error' });
      }
    } catch (e) {
      showAlert({ title: '网络请求错误', message: '扫描请求失败: ' + e, type: 'error' });
    } finally {
      btnScan.disabled = false;
      if (scanSpinner) scanSpinner.style.display = 'none';
      if (scanBtnText) scanBtnText.textContent = '🔍 重新扫描';
    }
  }

  function renderResult(summary) {
    if (!resultBox) return;
    resultBox.style.display = 'block';

    const count = summary.total_count || 0;
    const bytes = summary.total_orphan_bytes || 0;

    if (reclaimableSize) reclaimableSize.textContent = formatBytes(bytes);

    if (categoryGrid) {
      categoryGrid.innerHTML = `
        <div class="orphan-stat-pill">
          <span class="orphan-stat-label">临时分片缓存</span>
          <strong class="orphan-stat-val">${formatBytes(summary.temp_chunk_bytes || 0)}</strong>
        </div>
        <div class="orphan-stat-pill">
          <span class="orphan-stat-label">脱节源视频</span>
          <strong class="orphan-stat-val">${formatBytes(summary.unlinked_upload_bytes || 0)}</strong>
        </div>
        <div class="orphan-stat-pill">
          <span class="orphan-stat-label">脱节成品视频</span>
          <strong class="orphan-stat-val">${formatBytes(summary.unlinked_output_bytes || 0)}</strong>
        </div>
        <div class="orphan-stat-pill">
          <span class="orphan-stat-label">遗留转码残片</span>
          <strong class="orphan-stat-val">${formatBytes(summary.stale_partial_bytes || 0)}</strong>
        </div>
      `;
    }

    if (count > 0) {
      if (summaryText) summaryText.textContent = `共检测到 ${count} 个残留脱节文件与缓存分片`;
      if (btnClean) btnClean.style.display = 'inline-flex';
    } else {
      if (summaryText) summaryText.textContent = '✨ 存储环境良好，未发现任何残留文件';
      if (btnClean) btnClean.style.display = 'none';
    }
  }

  if (btnScan) {
    btnScan.addEventListener('click', performScan);
  }

  if (btnClean) {
    btnClean.addEventListener('click', async () => {
      if (!currentOrphanSummary || currentOrphanSummary.total_count === 0) return;
      const count = currentOrphanSummary.total_count;
      const sizeStr = formatBytes(currentOrphanSummary.total_orphan_bytes);

      const confirmed = await showConfirm({
        title: '确认彻底清理残留文件',
        message: `即将永久删除 ${count} 个未关联残留文件与临时缓存，预估释放 ${sizeStr} 磁盘空间。\n\n本操作将物理删除文件且不可恢复，是否确认清理？`,
        confirmText: '确认彻底清理',
        cancelText: '取消',
        type: 'danger',
        icon: '🧹',
      });

      if (!confirmed) return;

      btnClean.disabled = true;
      btnClean.textContent = '清理中...';

      try {
        const res = await apiFetch('/api/v1/system/orphans/clean', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({}),
        });

        if (res.ok) {
          const data = await res.json();
          showToast(`已成功清理 ${data.deleted_count || 0} 个残留文件，释放 ${formatBytes(data.freed_bytes || 0)} 空间！`, 'success', 4000);
          bus.emit('metrics:refresh');
          await performScan();
        } else if (res.status === 403) {
          showAlert({ title: '权限不足', message: '仅管理员有权执行物理文件清理操作', type: 'error' });
        } else {
          showAlert({ title: '清理失败', message: '执行清理失败，请检查服务端日志', type: 'error' });
        }
      } catch (e) {
        showAlert({ title: '网络请求错误', message: '清理请求失败: ' + e, type: 'error' });
      } finally {
        btnClean.disabled = false;
        btnClean.textContent = '🗑️ 彻底清理释放空间';
      }
    });
  }

  return {
    performScan,
    getSummary: () => currentOrphanSummary,
  };
}
