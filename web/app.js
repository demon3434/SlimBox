// SlimBox Web Dashboard Frontend Logic

let currentTask = null; // Recently uploaded task
let selectedProfile = null; // ADR-0003: Neutral selection, no default profile forced!
let profilesData = [];
let activePollingTimer = null;

document.addEventListener('DOMContentLoaded', () => {
  initStats();
  initProfiles();
  initUpload();
  initTabs();
  initTasksPolling();
  initSettingsModal();
});

// 1. Hardware Stats
async function initStats() {
  const updateStats = async () => {
    try {
      const res = await fetch('/api/v1/system/stats');
      if (!res.ok) return;
      const data = await res.json();
      
      document.getElementById('statCpu').textContent = `${data.num_cpu} 核活跃`;
      document.getElementById('statMem').textContent = `${data.memory_alloc_mb.toFixed(0)} MB`;
      document.getElementById('statDisk').textContent = `${data.disk_used_gb.toFixed(0)}G / ${data.disk_total_gb.toFixed(0)}G (${data.disk_usage_pct.toFixed(0)}%)`;
      document.getElementById('statDiskBar').style.width = `${Math.min(data.disk_usage_pct, 100)}%`;
    } catch (e) {
      console.warn('Failed to fetch system stats', e);
    }
  };

  updateStats();
  setInterval(updateStats, 4000);
}

// 2. Profiles Matrix (ADR-0003, ADR-0007)
async function initProfiles() {
  const grid = document.getElementById('profileGrid');
  const codecRadios = document.querySelectorAll('input[name="baseCodec"]');

  codecRadios.forEach(radio => {
    radio.addEventListener('change', () => {
      renderProfiles();
    });
  });

  try {
    const res = await fetch('/api/v1/profiles');
    if (res.ok) {
      profilesData = await res.json();
      renderProfiles();
    }
  } catch (e) {
    console.error('Failed to load profiles', e);
  }
}

function getSelectedBaseCodec() {
  const checked = document.querySelector('input[name="baseCodec"]:checked');
  return checked ? checked.value : 'libx265';
}

function renderProfiles() {
  const grid = document.getElementById('profileGrid');
  grid.innerHTML = '';
  const currentCodec = getSelectedBaseCodec();

  profilesData.forEach(p => {
    const card = document.createElement('div');
    card.className = `profile-card ${selectedProfile === p.name ? 'selected' : ''}`;
    card.dataset.name = p.name;

    const params = p.effective_params || p.default_params;
    const crf = currentCodec === 'libx265' ? params.crf : Math.max(18, params.crf - 1);

    card.innerHTML = `
      <div class="p-header">
        <span class="p-name">${p.label}</span>
        <span class="p-savings">${p.estimated_savings}</span>
      </div>
      <p class="p-desc">${p.description}</p>
      <div class="p-details">
        <span>${currentCodec === 'libx265' ? 'H.265' : 'H.264'} / CRF ${crf}</span>
        <span>${params.preset}</span>
        <span>音频 ${params.audio_bitrate}</span>
      </div>
      <div class="p-actions">
        <button class="p-action-btn btn-save" title="将当前配置保存为该档位自定预设">保存</button>
        ${p.is_customized ? `<button class="p-action-btn btn-reset" title="恢复出厂默认值">恢复</button>` : ''}
      </div>
    `;

    // Click card to select profile (Neutral selection)
    card.addEventListener('click', (e) => {
      if (e.target.classList.contains('p-action-btn')) return;
      selectProfile(p.name);
    });

    // Save custom profile button
    const saveBtn = card.querySelector('.btn-save');
    saveBtn.addEventListener('click', async (e) => {
      e.stopPropagation();
      const newParams = { ...params, video_codec: currentCodec, crf: crf };
      await saveCustomProfile(p.name, newParams);
    });

    // Reset profile button
    const resetBtn = card.querySelector('.btn-reset');
    if (resetBtn) {
      resetBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        await resetProfile(p.name);
      });
    }

    grid.appendChild(card);
  });
}

function selectProfile(name) {
  selectedProfile = name;
  document.querySelectorAll('.profile-card').forEach(c => {
    c.classList.toggle('selected', c.dataset.name === name);
  });

  const notice = document.getElementById('selectionNotice');
  notice.textContent = `已选定档位: ${name.toUpperCase()} (随时可更改)`;
  notice.style.color = 'var(--accent-cyan)';
  notice.style.background = 'rgba(45, 212, 191, 0.1)';

  checkSubmitButtonState();
}

async function saveCustomProfile(name, params) {
  try {
    const res = await fetch(`/api/v1/profiles/${name}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    if (res.ok) {
      alert(`已成功将预设保存到档位 [${name}]`);
      const ref = await fetch('/api/v1/profiles');
      profilesData = await ref.json();
      renderProfiles();
    }
  } catch (e) {
    alert('保存预设失败: ' + e);
  }
}

async function resetProfile(name) {
  try {
    const res = await fetch(`/api/v1/profiles/${name}/reset`, { method: 'POST' });
    if (res.ok) {
      const ref = await fetch('/api/v1/profiles');
      profilesData = await ref.json();
      renderProfiles();
    }
  } catch (e) {
    alert('重置预设失败: ' + e);
  }
}

// 3. File Upload & Media Probe
function initUpload() {
  const dropZone = document.getElementById('dropZone');
  const fileInput = document.getElementById('fileInput');

  dropZone.addEventListener('click', () => fileInput.click());
  fileInput.addEventListener('change', (e) => {
    if (e.target.files && e.target.files.length > 0) {
      handleFileUpload(e.target.files[0]);
    }
  });

  dropZone.addEventListener('dragover', (e) => {
    e.preventDefault();
    dropZone.classList.add('dragover');
  });

  dropZone.addEventListener('dragleave', () => {
    dropZone.classList.remove('dragover');
  });

  dropZone.addEventListener('drop', (e) => {
    e.preventDefault();
    dropZone.classList.remove('dragover');
    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      handleFileUpload(e.dataTransfer.files[0]);
    }
  });

  document.getElementById('submitQueueBtn').addEventListener('click', submitTaskToQueue);
}

function handleFileUpload(file) {
  const progContainer = document.getElementById('uploadProgressContainer');
  const progBar = document.getElementById('uploadProgressBar');
  const progText = document.getElementById('uploadProgressText');
  const probeCard = document.getElementById('probeCard');

  progContainer.style.display = 'block';
  progBar.style.width = '0%';
  progText.textContent = `上传中: ${file.name} (0%)`;
  probeCard.style.display = 'none';

  const formData = new FormData();
  formData.append('file', file);

  const xhr = new XMLHttpRequest();
  xhr.open('POST', '/api/v1/upload', true);

  xhr.upload.onprogress = (e) => {
    if (e.lengthComputable) {
      const pct = Math.round((e.loaded / e.total) * 100);
      progBar.style.width = `${pct}%`;
      progText.textContent = `上传中: ${pct}% (${formatBytes(e.loaded)} / ${formatBytes(e.total)})`;
    }
  };

  xhr.onload = () => {
    if (xhr.status >= 200 && xhr.status < 300) {
      progText.textContent = '上传完成，后台媒体分析中...';
      const task = JSON.parse(xhr.responseText);
      currentTask = task;
      displayProbeInfo(task);
      checkSubmitButtonState();
      setTimeout(() => { progContainer.style.display = 'none'; }, 1000);
    } else {
      progText.textContent = '上传失败: ' + xhr.responseText;
    }
  };

  xhr.onerror = () => {
    progText.textContent = '网络传输错误，请检查设备联通';
  };

  xhr.send(formData);
}

function displayProbeInfo(task) {
  const probeCard = document.getElementById('probeCard');
  const media = task.media_info;
  document.getElementById('probeFileName').textContent = task.source_file_name;

  if (media && media.video) {
    document.getElementById('probeVideoCodec').textContent = media.video.codec_name.toUpperCase();
    document.getElementById('probeResolution').textContent = `${media.video.width} x ${media.video.height}`;
    document.getElementById('probeFpsDuration').textContent = `${media.video.fps.toFixed(1)} fps / ${formatSeconds(media.duration_seconds)}`;
  } else {
    document.getElementById('probeVideoCodec').textContent = '通用媒体';
    document.getElementById('probeResolution').textContent = '自适应';
    document.getElementById('probeFpsDuration').textContent = '--';
  }

  document.getElementById('probeSize').textContent = formatBytes(task.source_file_size);

  // Audio summary
  const audioCount = (media && media.audio_tracks) ? media.audio_tracks.length : 0;
  const audioDetail = (media && media.audio_tracks && media.audio_tracks.length > 0)
    ? media.audio_tracks.map(a => `${a.codec_name} (${a.channel_layout || a.channels + 'ch'})`).join(', ')
    : '无音轨';
  document.getElementById('probeAudioSummary').textContent = `🎵 ${audioCount} 条音频轨: ${audioDetail}`;

  // Subtitle summary
  const subCount = (media && media.subtitle_tracks) ? media.subtitle_tracks.length : 0;
  document.getElementById('probeSubtitleSummary').textContent = `💬 ${subCount} 条内嵌软字幕 (直通保留)`;

  probeCard.style.display = 'block';
}

function checkSubmitButtonState() {
  const btn = document.getElementById('submitQueueBtn');
  const isTabAdvanced = document.getElementById('paneAdvanced').style.display !== 'none';
  
  if (currentTask && (selectedProfile || isTabAdvanced)) {
    btn.disabled = false;
  } else {
    btn.disabled = true;
  }
}

// 4. Tabs
function initTabs() {
  const tabPresetBtn = document.getElementById('tabPresetBtn');
  const tabAdvancedBtn = document.getElementById('tabAdvancedBtn');
  const panePreset = document.getElementById('panePreset');
  const paneAdvanced = document.getElementById('paneAdvanced');
  const crfSlider = document.getElementById('advCRF');
  const crfValLabel = document.getElementById('crfValLabel');

  crfSlider.addEventListener('input', (e) => {
    crfValLabel.textContent = e.target.value;
  });

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

// 5. Submit Task
async function submitTaskToQueue() {
  if (!currentTask) return;

  const isTabAdvanced = document.getElementById('paneAdvanced').style.display !== 'none';
  let payload = {};

  if (isTabAdvanced) {
    payload = {
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
      }
    };
  } else {
    if (!selectedProfile) {
      alert('请先点击选择一个分辨率档位！');
      return;
    }
    payload = {
      profile_name: selectedProfile,
      custom_params: {
        video_codec: getSelectedBaseCodec(),
      }
    };
  }

  try {
    const res = await fetch(`/api/v1/tasks/${currentTask.id}/start`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });

    if (res.ok) {
      // Clear current upload
      currentTask = null;
      document.getElementById('probeCard').style.display = 'none';
      document.getElementById('fileInput').value = '';
      checkSubmitButtonState();
      pollTasks();
    } else {
      const err = await res.json();
      alert('提交入队失败: ' + (err.message || '未知错误'));
    }
  } catch (e) {
    alert('请求失败: ' + e);
  }
}

// 6. Live Tasks Polling & Control
function initTasksPolling() {
  pollTasks();
  activePollingTimer = setInterval(pollTasks, 1500);

  document.getElementById('refreshQueueBtn').addEventListener('click', pollTasks);
  document.getElementById('abortActiveBtn').addEventListener('click', abortActiveTask);
}

async function pollTasks() {
  try {
    const res = await fetch('/api/v1/tasks');
    if (!res.ok) return;
    const data = await res.json();
    const tasks = data.tasks || [];

    const active = tasks.find(t => t.status === 'transcoding');
    renderActiveTask(active);

    const queued = tasks.filter(t => t.status === 'queued' || t.status === 'pending');
    renderQueueList(queued);

    const completed = tasks.filter(t => t.status === 'completed' || t.status === 'failed' || t.status === 'aborted');
    renderHistoryList(completed);
  } catch (e) {
    console.warn('Tasks poll failed', e);
  }
}

function renderActiveTask(task) {
  const noActiveMsg = document.getElementById('noActiveTaskMsg');
  const activeContent = document.getElementById('activeTaskContent');

  if (!task) {
    noActiveMsg.style.display = 'block';
    activeContent.style.display = 'none';
    return;
  }

  noActiveMsg.style.display = 'none';
  activeContent.style.display = 'block';

  document.getElementById('activeTaskFileName').textContent = task.source_file_name;
  document.getElementById('activeTaskProfile').textContent = (task.params && task.params.profile_name) 
    ? `${task.params.profile_name.toUpperCase()} (${task.params.video_codec})` 
    : '正在转码';

  const prog = task.progress || {};
  const pct = prog.percent ? prog.percent.toFixed(1) : '0.0';
  document.getElementById('activeProgressBar').style.width = `${pct}%`;
  document.getElementById('activeProgressPct').textContent = `${pct}%`;

  const fps = prog.current_fps ? prog.current_fps.toFixed(1) : '0';
  const speed = prog.speed || '1.0x';
  document.getElementById('activeSpeedFps').textContent = `${fps} fps (${speed})`;
  document.getElementById('activeTime').textContent = prog.current_time_str || '00:00:00';

  if (prog.eta_seconds && prog.eta_seconds > 0) {
    document.getElementById('activeETA').textContent = formatSeconds(prog.eta_seconds);
  } else {
    document.getElementById('activeETA').textContent = '计算中...';
  }

  document.getElementById('abortActiveBtn').dataset.taskId = task.id;
}

async function abortActiveTask() {
  const taskId = document.getElementById('abortActiveBtn').dataset.taskId;
  if (!taskId) return;

  if (!confirm('⚠️ 确定强行终止当前正在运行的转码任务吗？\n系统将立即释放 CPU 资源并物理删除半成品残片文件 (ADR-0008, ADR-0011)。')) {
    return;
  }

  try {
    const res = await fetch(`/api/v1/tasks/${taskId}/abort`, { method: 'POST' });
    if (res.ok) {
      pollTasks();
    } else {
      alert('中止任务失败');
    }
  } catch (e) {
    alert('请求失败: ' + e);
  }
}

function renderQueueList(tasks) {
  const container = document.getElementById('queueList');
  document.getElementById('queueCount').textContent = tasks.length;
  container.innerHTML = '';

  if (tasks.length === 0) {
    container.innerHTML = `<div style="text-align:center; padding: 1rem; color: var(--text-dim); font-size: 0.8rem;">队列中暂无等待任务</div>`;
    return;
  }

  tasks.forEach(t => {
    const item = document.createElement('div');
    item.className = 'task-item';
    const profile = (t.params && t.params.profile_name) ? t.params.profile_name : '未配置';
    item.innerHTML = `
      <div class="task-item-left">
        <span class="task-item-title">${t.source_file_name}</span>
        <span class="task-item-sub">档位: ${profile} | 大小: ${formatBytes(t.source_file_size)}</span>
      </div>
      <div class="task-item-right">
        <button class="btn btn-sm btn-ghost btn-cancel" title="取消排队">✕ 移除</button>
      </div>
    `;

    item.querySelector('.btn-cancel').addEventListener('click', async () => {
      await fetch(`/api/v1/tasks/${t.id}`, { method: 'DELETE' });
      pollTasks();
    });

    container.appendChild(item);
  });
}

function renderHistoryList(tasks) {
  const container = document.getElementById('historyList');
  document.getElementById('historyCount').textContent = tasks.length;
  container.innerHTML = '';

  if (tasks.length === 0) {
    container.innerHTML = `<div style="text-align:center; padding: 1rem; color: var(--text-dim); font-size: 0.8rem;">暂无历史完成任务</div>`;
    return;
  }

  tasks.forEach(t => {
    const item = document.createElement('div');
    item.className = 'task-item';

    let statusBadge = '';
    let actionBtn = '';

    if (t.status === 'completed') {
      const orig = t.source_file_size;
      const comp = t.output_file_size;
      const savings = orig > 0 ? Math.round((1 - comp / orig) * 100) : 0;
      statusBadge = `<span class="savings-tag">缩减 ${savings}% (${formatBytes(orig)} -> ${formatBytes(comp)})</span>`;
      actionBtn = `<a href="/api/v1/tasks/${t.id}/download" class="btn btn-sm btn-primary" download>⬇️ 下载</a>`;
    } else if (t.status === 'failed') {
      statusBadge = `<span style="color: var(--accent-danger); font-size: 0.75rem;">转码失败 (残片已清理)</span>`;
    } else if (t.status === 'aborted') {
      statusBadge = `<span style="color: var(--text-dim); font-size: 0.75rem;">用户中止 (残片已清理)</span>`;
    }

    item.innerHTML = `
      <div class="task-item-left">
        <span class="task-item-title">${t.source_file_name}</span>
        <span class="task-item-sub">${statusBadge}</span>
      </div>
      <div class="task-item-right">
        ${actionBtn}
        <button class="btn btn-sm btn-ghost btn-del" title="删除记录与文件">🗑</button>
      </div>
    `;

    item.querySelector('.btn-del').addEventListener('click', async () => {
      if (confirm(`确定删除任务 ${t.source_file_name} 及其相关文件吗？`)) {
        await fetch(`/api/v1/tasks/${t.id}`, { method: 'DELETE' });
        pollTasks();
      }
    });

    container.appendChild(item);
  });
}

// 7. Settings Modal
function initSettingsModal() {
  const modal = document.getElementById('settingsModal');
  const openBtn = document.getElementById('openSettingsBtn');
  const closeBtn = document.getElementById('closeSettingsBtn');
  const saveBtn = document.getElementById('saveSettingsBtn');

  openBtn.addEventListener('click', async () => {
    modal.style.display = 'flex';
    try {
      const res = await fetch('/api/v1/system/settings');
      if (res.ok) {
        const s = await res.json();
        document.getElementById('setDeleteSource').checked = !!s.delete_source_after_transcode;
        document.getElementById('setDeleteOutput').checked = !!s.delete_output_after_download;
        document.getElementById('setRetention').value = s.retention_hours || 0;
      }
    } catch (e) {
      console.warn('Failed to load settings', e);
    }
  });

  closeBtn.addEventListener('click', () => modal.style.display = 'none');

  saveBtn.addEventListener('click', async () => {
    const payload = {
      delete_source_after_transcode: document.getElementById('setDeleteSource').checked,
      delete_output_after_download: document.getElementById('setDeleteOutput').checked,
      retention_hours: parseInt(document.getElementById('setRetention').value) || 0,
    };

    try {
      const res = await fetch('/api/v1/system/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (res.ok) {
        alert('存储生命周期配置已成功更新！');
        modal.style.display = 'none';
      }
    } catch (e) {
      alert('保存设置失败: ' + e);
    }
  });
}

// Utilities
function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatSeconds(sec) {
  const s = Math.round(sec);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const seconds = s % 60;
  if (h > 0) {
    return `${h}时${m}分${seconds}秒`;
  }
  return `${m}分${seconds}秒`;
}
