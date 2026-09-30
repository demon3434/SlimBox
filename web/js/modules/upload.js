import { bus } from '../bus.js';
import { getAuthToken, apiFetch } from '../api.js';
import { formatBytes, formatSeconds } from '../utils.js';
import { getSelectedProfile, getSelectedProfilePreset, getSelectedBaseCodec } from './profiles.js';
import { showAlert, showToast } from './dialog.js';
import {
  getStagedTasks,
  getSelectedStagedTaskIds,
  removeStagedTaskIds,
  addStagedTask,
  clearStagedTasksInMemory,
  updateEnqueueButtonState,
  initStagedBatch,
  displayProbeInfo
} from './upload_staged.js';

export function getCurrentTask() {
  const list = getStagedTasks();
  return list.length > 0 ? list[list.length - 1] : null;
}

export function setCurrentTask(task) {
  if (task) addStagedTask(task);
  checkSubmitButtonState();
}

export function checkSubmitButtonState() {
  updateEnqueueButtonState();
}

let chunkThresholdBytes = 200 * 1024 * 1024; // default 200MB threshold
const CHUNK_SIZE_BYTES = 20 * 1024 * 1024; // 20MB chunks

export function setChunkThresholdMB(mb) {
  if (mb > 0) {
    chunkThresholdBytes = mb * 1024 * 1024;
  }
}

export async function fetchCurrentChunkThreshold() {
  try {
    const res = await apiFetch('/api/v1/system/settings');
    if (res.ok) {
      const s = await res.json();
      if (s.chunk_threshold_mb && s.chunk_threshold_mb > 0) {
        setChunkThresholdMB(s.chunk_threshold_mb);
      }
    }
  } catch (_) {}
}

let uploadQueue = [];
let isUploading = false;

const VIDEO_EXTS = new Set([
  '.mp4', '.mkv', '.avi', '.mov', '.flv', '.ts', '.wmv', '.webm', '.m4v', '.iso', '.rmvb', '.vob', '.m2ts'
]);

function isVideoFile(file) {
  if (file.type && file.type.startsWith('video/')) return true;
  const dotIdx = file.name.lastIndexOf('.');
  if (dotIdx === -1) return true;
  return VIDEO_EXTS.has(file.name.substring(dotIdx).toLowerCase());
}

export function handleFiles(fileList) {
  if (!fileList || fileList.length === 0) return;
  const rawFiles = Array.from(fileList);
  const videoFiles = rawFiles.filter(isVideoFile);
  const ignoredCount = rawFiles.length - videoFiles.length;

  if (ignoredCount > 0) {
    showToast(`已自动忽略 ${ignoredCount} 个非视频文件`, 'info', 3000);
  }

  if (videoFiles.length === 0) {
    if (rawFiles.length > 0) {
      showAlert({
        title: '未检测到视频文件',
        message: '请选择或拖拽 MKV, MP4, AVI, TS, MOV 等有效视频格式文件。',
        type: 'warning',
      });
    }
    return;
  }

  uploadQueue.push(...videoFiles);
  if (!isUploading) {
    processUploadQueue();
  }
}

export function handleFileUpload(file) {
  if (file) handleFiles([file]);
}

async function processUploadQueue() {
  if (isUploading || uploadQueue.length === 0) return;
  isUploading = true;

  const progContainer = document.getElementById('uploadProgressContainer');
  const progBar = document.getElementById('uploadProgressBar');
  const progText = document.getElementById('uploadProgressText');
  const probeCard = document.getElementById('probeCard');

  if (progContainer) progContainer.style.display = 'block';
  if (probeCard) probeCard.style.display = 'none';

  const totalBatch = uploadQueue.length;
  let successCount = 0;
  let failCount = 0;
  let currentIndex = 0;

  while (uploadQueue.length > 0) {
    const file = uploadQueue.shift();
    currentIndex++;
    const batchInfo = { current: currentIndex, total: totalBatch };

    try {
      let task;
      if (file.size > chunkThresholdBytes) {
        task = await uploadChunkedFileAsync(file, batchInfo);
      } else {
        task = await uploadSingleFileAsync(file, batchInfo);
      }

      successCount++;
      addStagedTask(task);
      displayProbeInfo(task);
      checkSubmitButtonState();
    } catch (err) {
      failCount++;
      showToast(`文件 [${file.name}] 上传失败: ${err.message}`, 'danger', 4500);
    }
  }

  isUploading = false;
  if (totalBatch > 1) {
    if (failCount === 0) {
      showToast(`批量上传完成: 共 ${successCount} 个视频已就绪，请在下方选择压缩参数后加入队列`, 'success', 4000);
      if (progText) progText.textContent = `全部 ${successCount} 个视频上传完成，待配置！`;
    } else {
      showToast(`批量上传结束: ${successCount} 成功, ${failCount} 失败`, 'info', 4000);
      if (progText) progText.textContent = `批量结束: ${successCount} 成功, ${failCount} 失败`;
    }
  } else if (successCount === 1) {
    showToast('视频上传完成，请在下方选择压缩档位后加入队列', 'success', 2500);
    if (progText) progText.textContent = '视频上传与探测完成！';
  }

  const fileInput = document.getElementById('fileInput');
  if (fileInput) fileInput.value = '';

  setTimeout(() => {
    if (!isUploading && progContainer) {
      progContainer.style.display = 'none';
      if (progBar) progBar.style.width = '0%';
    }
  }, 1200);
}

function uploadSingleFileAsync(file, batchInfo) {
  return new Promise((resolve, reject) => {
    const progBar = document.getElementById('uploadProgressBar');
    const progText = document.getElementById('uploadProgressText');
    const prefix = batchInfo && batchInfo.total > 1 ? `[${batchInfo.current}/${batchInfo.total}] ` : '';

    if (progBar) progBar.style.width = '0%';
    if (progText) progText.textContent = `${prefix}上传中: ${file.name} (0%)`;

    const formData = new FormData();
    formData.append('file', file);

    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/api/v1/upload', true);

    const token = getAuthToken();
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`);

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        const pct = Math.round((e.loaded / e.total) * 100);
        if (progBar) progBar.style.width = `${pct}%`;
        if (progText) progText.textContent = `${prefix}上传中: ${file.name} ${pct}% (${formatBytes(e.loaded)} / ${formatBytes(e.total)})`;
      }
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        if (progText) progText.textContent = `${prefix}上传完成，后台分析媒体流中...`;
        try {
          const task = JSON.parse(xhr.responseText);
          resolve(task);
        } catch (_) {
          reject(new Error('解析服务端响应失败'));
        }
      } else {
        reject(new Error(xhr.responseText || `HTTP ${xhr.status}`));
      }
    };

    xhr.onerror = () => reject(new Error('网络传输错误，请检查设备连通性'));
    xhr.send(formData);
  });
}

async function uploadChunkedFileAsync(file, batchInfo) {
  const progBar = document.getElementById('uploadProgressBar');
  const progText = document.getElementById('uploadProgressText');
  const prefix = batchInfo && batchInfo.total > 1 ? `[${batchInfo.current}/${batchInfo.total}] ` : '';

  if (progBar) progBar.style.width = '0%';
  if (progText) progText.textContent = `${prefix}准备分片上传: ${file.name} (${formatBytes(file.size)})...`;

  const uploadId = 'up_' + Date.now() + '_' + Math.random().toString(36).substring(2, 9);
  const totalChunks = Math.ceil(file.size / CHUNK_SIZE_BYTES);
  const token = getAuthToken();

  for (let chunkIndex = 0; chunkIndex < totalChunks; chunkIndex++) {
    const start = chunkIndex * CHUNK_SIZE_BYTES;
    const end = Math.min(start + CHUNK_SIZE_BYTES, file.size);
    const chunkBlob = file.slice(start, end);

    await uploadSingleChunkWithRetry(uploadId, chunkIndex, totalChunks, chunkBlob, token, (attempt) => {
      if (attempt > 1 && progText) {
        progText.textContent = `${prefix}分片 ${chunkIndex + 1}/${totalChunks} 传输抖动，正在第 ${attempt} 次重试...`;
      }
    });

    const loadedBytes = end;
    const pct = Math.round((loadedBytes / file.size) * 100);
    if (progBar) progBar.style.width = `${pct}%`;
    if (progText) progText.textContent = `${prefix}大文件分片上传中: ${pct}% (分片 ${chunkIndex + 1}/${totalChunks} · ${formatBytes(loadedBytes)} / ${formatBytes(file.size)})`;
  }

  if (progText) progText.textContent = `${prefix}所有分片传输完毕，流式合并与探测中...`;

  const completeRes = await apiFetch('/api/v1/upload/complete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      upload_id: uploadId,
      filename: file.name,
      total_chunks: totalChunks,
    }),
  });

  if (!completeRes.ok) {
    const errText = await completeRes.text();
    throw new Error(`分片合并失败: ${errText}`);
  }

  return await completeRes.json();
}

async function uploadSingleChunkWithRetry(uploadId, chunkIndex, totalChunks, chunkBlob, token, onRetry, maxRetries = 3) {
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    if (onRetry) onRetry(attempt);
    try {
      const formData = new FormData();
      formData.append('upload_id', uploadId);
      formData.append('chunk_index', String(chunkIndex));
      formData.append('total_chunks', String(totalChunks));
      formData.append('chunk', chunkBlob, 'blob');

      const res = await apiFetch('/api/v1/upload/chunk', {
        method: 'POST',
        body: formData,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(`HTTP ${res.status}: ${text}`);
      }
      return true;
    } catch (err) {
      if (attempt === maxRetries) {
        throw new Error(`分片 ${chunkIndex + 1} 上传失败 (已重试 ${maxRetries} 次): ${err.message}`);
      }
      await new Promise((resolve) => setTimeout(resolve, attempt * 800));
    }
  }
}

export async function submitTaskToQueue() {
  const selectedIds = getSelectedStagedTaskIds();
  if (selectedIds.length === 0) {
    showAlert({ title: '请勾选视频', message: '请至少勾选一个待配置视频后再提交转码。', type: 'warning', icon: '🎬' });
    return;
  }

  const isTabAdvanced = document.getElementById('paneAdvanced')?.style.display !== 'none';
  let payload = {};

  if (isTabAdvanced) {
    payload = {
      task_ids: selectedIds,
      profile_name: 'custom',
      custom_params: {
        video_codec: document.getElementById('advCodec').value,
        target_resolution: document.getElementById('advRes').value,
        crf: parseInt(document.getElementById('advCRF').value),
        preset: document.getElementById('advPreset').value,
        audio_codec: 'aac',
        audio_bitrate: document.getElementById('advAudioBitrate').value,
        audio_track_policy: document.getElementById('advAudioPolicy').value,
        subtitle_policy: document.getElementById('advSubtitlePolicy').value,
        extra_args: document.getElementById('advExtra').value.trim(),
        faststart: document.getElementById('advFastStart').checked,
        keyframe_interval: parseInt(document.getElementById('advKeyframe').value),
        max_fps: parseInt(document.getElementById('advMaxFPS')?.value || '30', 10),
      }
    };
  } else {
    const selectedProfile = getSelectedProfile();
    if (!selectedProfile) {
      showAlert({ title: '请选择分辨率档位', message: '请先在预设面板中点击选择一个目标分辨率档位（如 720p / 1080p），然后再提交转码。', type: 'warning', icon: '🎯' });
      return;
    }
    const currentPreset = getSelectedProfilePreset();
    const presetParams = currentPreset ? (currentPreset.effective_params || currentPreset.default_params) : null;
    const fastStartVal = presetParams ? (presetParams.faststart !== false) : true;
    const maxFpsVal = presetParams && presetParams.max_fps !== undefined ? presetParams.max_fps : 30;

    payload = {
      task_ids: selectedIds,
      profile_name: selectedProfile,
      custom_params: {
        video_codec: getSelectedBaseCodec(),
        faststart: fastStartVal,
        max_fps: maxFpsVal,
      }
    };
  }

  try {
    const res = await apiFetch('/api/v1/tasks/batch-start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });

    if (res.ok) {
      const count = selectedIds.length;
      removeStagedTaskIds(selectedIds);
      const remaining = getStagedTasks().length;
      if (remaining > 0) {
        showToast(`已将 ${count} 个视频加入队列，剩余 ${remaining} 个视频待配置`, 'success');
      } else {
        const fileInput = document.getElementById('fileInput');
        if (fileInput) fileInput.value = '';
        showToast(`已成功将 ${count} 个视频加入排队队列，开始转码`, 'success');
      }
      bus.emit('task:created');
    } else {
      const err = await res.json().catch(() => ({}));
      showAlert({ title: '提交入队失败', message: '提交入队失败: ' + (err.message || '未知错误'), type: 'error' });
    }
  } catch (e) {
    showAlert({ title: '网络请求错误', message: '请求失败: ' + e, type: 'error' });
  }
}

export function initTabs() {
  const tabPresetBtn = document.getElementById('tabPresetBtn');
  const tabAdvancedBtn = document.getElementById('tabAdvancedBtn');
  const panePreset = document.getElementById('panePreset');
  const paneAdvanced = document.getElementById('paneAdvanced');
  const crfSlider = document.getElementById('advCRF');
  const crfValLabel = document.getElementById('crfValLabel');
  const advCodec = document.getElementById('advCodec');
  const updateAdvQualityLabels = () => {
    const codec = advCodec ? advCodec.value : 'hevc_videotoolbox';
    const isApple = codec === 'hevc_videotoolbox' || codec === 'h264_videotoolbox';
    const val = parseInt(crfSlider?.value || '24', 10);
    const qVal = Math.max(20, Math.min(95, 50 - (val - 24) * 2));

    const titleEl = document.getElementById('advQualityTitle');
    const advPresetGroup = document.getElementById('advPresetGroup');
    const presetEl = document.getElementById('advPreset');

    if (isApple) {
      if (titleEl) titleEl.innerHTML = `Apple 硬件质量系数: <strong id="crfValLabel">Q${qVal}</strong> <span class="hint" id="advQualityHint">(底层 -q:v，Q越高清越大，推荐 Q50-Q70)</span>`;
      if (advPresetGroup) advPresetGroup.style.display = 'none';
    } else {
      if (titleEl) titleEl.innerHTML = `CRF 质量因子: <strong id="crfValLabel">${val}</strong> <span class="hint" id="advQualityHint">(值越大文件越小，18-32)</span>`;
      if (advPresetGroup) advPresetGroup.style.display = 'flex';
      if (presetEl) presetEl.disabled = false;
    }
  };

  if (advCodec) {
    advCodec.addEventListener('change', updateAdvQualityLabels);
  }
  if (crfSlider) {
    crfSlider.addEventListener('input', updateAdvQualityLabels);
  }
  updateAdvQualityLabels();

  if (tabPresetBtn && tabAdvancedBtn && panePreset && paneAdvanced) {
    tabPresetBtn.addEventListener('click', () => {
      tabPresetBtn.classList.add('active');
      tabAdvancedBtn.classList.remove('active');
      panePreset.style.display = 'block';
      paneAdvanced.style.display = 'none';
      checkSubmitButtonState();
    });

    tabAdvancedBtn.addEventListener('click', () => {
      tabAdvancedBtn.classList.add('active');
      tabPresetBtn.classList.remove('active');
      panePreset.style.display = 'none';
      paneAdvanced.style.display = 'block';
      checkSubmitButtonState();
    });
  }
}

export function initUpload() {
  const dropZone = document.getElementById('dropZone');
  const fileInput = document.getElementById('fileInput');

  // 全局防止误拖拽导致浏览器直接播放/跳出页面
  ['dragover', 'drop'].forEach((evt) => {
    window.addEventListener(evt, (e) => e.preventDefault(), false);
  });

  if (dropZone && fileInput) {
    dropZone.addEventListener('click', () => fileInput.click());

    fileInput.addEventListener('change', (e) => {
      if (e.target.files && e.target.files.length > 0) handleFiles(e.target.files);
    });

    const titleEl = dropZone.querySelector('.drop-title');
    const hintEl = dropZone.querySelector('.drop-hint');
    const origTitle = titleEl ? titleEl.innerHTML : '';
    const origHint = hintEl ? hintEl.textContent : '';

    dropZone.addEventListener('dragover', (e) => {
      e.preventDefault();
      e.stopPropagation();
      if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
      dropZone.classList.add('dragover');
      if (titleEl) titleEl.innerHTML = '✨ <strong>松开鼠标</strong>，立即加入待配置批次';
      if (hintEl) hintEl.textContent = '松手即刻开始媒体探测并就绪';
    });

    const resetDropText = () => {
      dropZone.classList.remove('dragover');
      if (titleEl && origTitle) titleEl.innerHTML = origTitle;
      if (hintEl && origHint) hintEl.textContent = origHint;
    };

    dropZone.addEventListener('dragleave', (e) => {
      e.preventDefault();
      e.stopPropagation();
      resetDropText();
    });

    dropZone.addEventListener('drop', (e) => {
      e.preventDefault();
      e.stopPropagation();
      resetDropText();
      if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
        handleFiles(e.dataTransfer.files);
      }
    });
  }

  const submitBtn = document.getElementById('submitQueueBtn');
  if (submitBtn) {
    submitBtn.addEventListener('click', submitTaskToQueue);
  }

  initTabs();
  initStagedBatch();

  // Listen to profile selections to update submit button disabled state
  bus.on('profile:selected', () => {
    checkSubmitButtonState();
  });

  fetchCurrentChunkThreshold();
  bus.on('settings:updated', (s) => {
    if (s && s.chunk_threshold_mb) {
      setChunkThresholdMB(s.chunk_threshold_mb);
    }
  });
}
