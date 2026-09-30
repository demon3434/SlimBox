import { bus } from '../bus.js';
import { apiFetch } from '../api.js';
import { showConfirm, showAlert, showToast } from './dialog.js';

let selectedProfile = null; // Neutral selection, no default profile forced!
let profilesData = [];
let currentEditingProfile = null;

export function calculateEstimatedSavings(params, currentCodec) {
  const res = (params.target_resolution || params.profile_name || '720p').toLowerCase();
  let baseSavings = 72;
  if (res.includes('360')) baseSavings = 88;
  else if (res.includes('480')) baseSavings = 80;
  else if (res.includes('720')) baseSavings = 72;
  else if (res.includes('1080')) baseSavings = 60;
  else if (res.includes('2k') || res.includes('1440')) baseSavings = 50;
  else if (res.includes('4k') || res.includes('2160')) baseSavings = 40;

  let remainingRatio = (100 - baseSavings) / 100.0;
  const crf = Number(params.crf) || 24;
  const baseCrf = res.includes('360') ? 26 : (res.includes('480') ? 25 : (res.includes('4k') ? 25 : 24));
  const crfFactor = Math.pow(2, (baseCrf - crf) / 6.0);
  remainingRatio *= crfFactor;

  const codec = currentCodec || params.video_codec || 'libx265';
  if (codec === 'libx264' || codec === 'h264' || codec === 'h264_videotoolbox') remainingRatio *= 1.35;
  else if (codec === 'hevc_videotoolbox') remainingRatio *= 1.12;

  const preset = (params.preset || 'fast').toLowerCase();
  if (preset === 'veryslow' || preset === 'slower') remainingRatio *= 0.92;
  else if (preset === 'slow') remainingRatio *= 0.96;
  else if (preset === 'veryfast') remainingRatio *= 1.05;
  else if (preset === 'superfast' || preset === 'ultrafast') remainingRatio *= 1.12;

  const audioBitrate = parseInt(params.audio_bitrate || '128k', 10);
  if (audioBitrate <= 64) remainingRatio *= 0.97;
  else if (audioBitrate >= 192) remainingRatio *= 1.03;

  remainingRatio = Math.max(0.03, Math.min(0.90, remainingRatio));
  const savingsPct = Math.round((1.0 - remainingRatio) * 100);
  return `${savingsPct}%⬇`;
}

function refreshModalSavings() {
  if (!currentEditingProfile) return;
  const currentCodec = getSelectedBaseCodec();
  const crf = parseInt(document.getElementById('modalCrf')?.value || '24', 10);
  const preset = document.getElementById('modalPreset')?.value || 'fast';
  const audioBitrate = document.getElementById('modalAudioBitrate')?.value || '128k';
  const est = calculateEstimatedSavings({
    target_resolution: currentEditingProfile.name,
    profile_name: currentEditingProfile.name,
    crf,
    preset,
    audio_bitrate: audioBitrate,
  }, currentCodec);
  const badge = document.getElementById('modalEstimatedSavings');
  if (badge) badge.textContent = est;
}

export function getSelectedProfile() {
  if (selectedProfile) return selectedProfile;
  const card = document.querySelector('.profile-card.selected');
  return card?.dataset?.name || null;
}

export function getProfilesData() {
  return profilesData;
}

export function getSelectedProfilePreset() {
  const cur = getSelectedProfile();
  return profilesData.find((p) => p.name === cur) || null;
}

export function getSelectedBaseCodec() {
  const checked = document.querySelector('input[name="baseCodec"]:checked');
  return checked ? checked.value : 'libx265';
}

export async function loadProfiles() {
  try {
    const res = await apiFetch('/api/v1/profiles');
    if (res.ok) {
      profilesData = await res.json();
      renderProfiles();
    }
  } catch (e) {
    console.error('Failed to load profiles', e);
  }
}

export function renderProfiles() {
  const grid = document.getElementById('profileGrid');
  if (!grid) return;
  grid.innerHTML = '';
  const currentCodec = getSelectedBaseCodec();

  profilesData.forEach((p) => {
    const card = document.createElement('div');
    card.className = `profile-card ${selectedProfile === p.name ? 'selected' : ''}`;
    card.dataset.name = p.name;

    const params = p.effective_params || p.default_params || {};
    let crf = params.crf || 24;
    if (currentCodec === 'libx264' || currentCodec === 'h264_videotoolbox') {
      crf = Math.max(18, crf - 1);
    }
    const keyInterval = params.keyframe_interval || 2;
    const isFastStart = params.faststart !== false;
    const isDropSub = params.subtitle_policy === 'drop';
    const qVal = Math.max(20, Math.min(95, 50 - (crf - 24) * 2));
    const isAppleHardware = currentCodec === 'hevc_videotoolbox' || currentCodec === 'h264_videotoolbox';

    const cMap = {
      hevc_videotoolbox: { l: 'Apple H.265', q: `Q${qVal} (VBR)` },
      h264_videotoolbox: { l: 'Apple H.264', q: `Q${qVal} (VBR)` },
      hevc_nvenc: { l: 'NVIDIA H.265', q: `CQ ${crf} (VBR)` },
      h264_nvenc: { l: 'NVIDIA H.264', q: `CQ ${crf} (VBR)` },
      hevc_qsv: { l: 'Intel QSV', q: `ICQ ${crf}` },
      h264_qsv: { l: 'Intel H.264', q: `ICQ ${crf}` },
      hevc_amf: { l: 'AMD H.265', q: `QP ${crf}` },
      h264_amf: { l: 'AMD H.264', q: `QP ${crf}` },
      hevc_vaapi: { l: 'VAAPI H.265', q: `QP ${crf}` },
      h264_vaapi: { l: 'VAAPI H.264', q: `QP ${crf}` },
      libx264: { l: 'H.264', q: `CRF ${crf}` },
    };
    const cInfo = cMap[currentCodec] || { l: 'H.265', q: `CRF ${crf}` };
    const codecLabel = cInfo.l;
    const qualityDisplay = cInfo.q;

    const savings = calculateEstimatedSavings(
      { ...params, crf, target_resolution: p.name, profile_name: p.name },
      currentCodec
    );

    card.innerHTML = `
      <div class="p-header">
        <span class="p-name">${p.label}</span>
        <span class="p-savings" title="预估体积缩减约 ${savings} (根据当前分辨率/画质系数/编码预设实时计算)">${savings}</span>
      </div>
      <p class="p-desc">${p.description}</p>
      <div class="p-details">
        <div class="p-params-row">
          <span>${codecLabel}</span>
          <span>${qualityDisplay}</span>
          ${isAppleHardware ? '' : `<span>${params.preset || 'fast'}</span>`}
          <span>音频 ${params.audio_bitrate || '128k'}</span>
        </div>
        <div class="p-features-row">
          <span class="p-tag-seek" title="关键帧寻道间隔 (Seek 秒开)">⏱️ ${keyInterval}s 关键帧</span>
          ${params.max_fps === 24
            ? `<span class="p-tag-fps" style="background: rgba(168, 85, 247, 0.15); color: #c084fc; border: 1px solid rgba(168, 85, 247, 0.3); padding: 2px 6px; border-radius: 4px; font-size: 0.75rem;" title="锁定 24 FPS 电影质感">🎞️ 24FPS锁定</span>`
            : (params.max_fps && params.max_fps > 0)
              ? `<span class="p-tag-fps" style="background: rgba(59, 130, 246, 0.15); color: #60a5fa; border: 1px solid rgba(59, 130, 246, 0.3); padding: 2px 6px; border-radius: 4px; font-size: 0.75rem;" title="智能帧率封顶 (60帧转${params.max_fps}帧，<=${params.max_fps}帧保持)">🎞️ ${params.max_fps}FPS封顶</span>`
              : `<span class="p-tag-fps" style="background: rgba(148, 163, 184, 0.12); color: #94a3b8; border: 1px solid rgba(148, 163, 184, 0.25); padding: 2px 6px; border-radius: 4px; font-size: 0.75rem;" title="保持原片帧率，不作抽帧限制">🎞️ 原片帧率</span>`
          }
          ${isFastStart ? `<span class="p-tag-faststart" title="MOOV Atom 前置 (边下边播秒开)">⚡ MOOV前置</span>` : ''}
          ${isDropSub ? `<span class="p-tag-dropsub" style="background: rgba(239, 68, 68, 0.15); color: #f87171; border: 1px solid rgba(239, 68, 68, 0.3); padding: 2px 6px; border-radius: 4px; font-size: 0.75rem;" title="丢弃所有字幕流">🚫 丢弃字幕</span>` : ''}
        </div>
      </div>
      <div class="p-actions">
        <button class="p-action-btn btn-edit" title="修改此档预设并保存到数据库"><span class="icon">✏️</span> 修改</button>
        <button class="p-action-btn btn-reset ${p.is_customized ? 'active' : 'disabled'}" title="${p.is_customized ? '重置为出厂默认设置' : '当前已是出厂默认配置'}"><span class="icon">↺</span> 重置</button>
      </div>
    `;

    // Click card to select profile
    card.addEventListener('click', (e) => {
      if (e.target.closest('.p-action-btn')) return;
      selectProfile(p.name);
    });

    // Edit button opens modal
    const editBtn = card.querySelector('.btn-edit');
    if (editBtn) {
      editBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        openPresetEditModal(p);
      });
    }

    // Reset profile button
    const resetBtn = card.querySelector('.btn-reset');
    if (resetBtn) {
      resetBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        if (!p.is_customized) {
          showAlert({
            title: '配置无需重置',
            message: `[${p.label}] 当前已经是出厂默认配置，无需重置。`,
            type: 'info',
            icon: 'ℹ️',
          });
          return;
        }
        const confirmed = await showConfirm({
          title: '重置预设档位',
          message: `确定要将 [${p.label}] 重置为出厂默认设置吗？\n当前保存的个性化配置将被清除。`,
          confirmText: '确认重置',
          cancelText: '取消',
          type: 'warning',
          icon: '🔄',
        });
        if (confirmed) {
          await resetProfile(p.name);
          showToast(`已恢复 [${p.label}] 为出厂默认配置`, 'success');
        }
      });
    }

    grid.appendChild(card);
  });
}

export function selectProfile(name) {
  selectedProfile = name;
  document.querySelectorAll('.profile-card').forEach((c) => {
    c.classList.toggle('selected', c.dataset.name === name);
  });

  bus.emit('profile:selected', name);
}

export function openPresetEditModal(profile) {
  currentEditingProfile = profile;
  const currentCodec = getSelectedBaseCodec();
  const params = profile.effective_params || profile.default_params;
  const crf = (currentCodec === 'libx264' || currentCodec === 'h264_videotoolbox') ? Math.max(18, params.crf - 1) : params.crf;

  const isApple = currentCodec === 'hevc_videotoolbox' || currentCodec === 'h264_videotoolbox';
  const qVal = Math.max(20, Math.min(95, 50 - (crf - 24) * 2));

  const titleEl = document.getElementById('presetModalTitle');
  if (titleEl) titleEl.textContent = `✏️ 修改 [${profile.label}] 预设参数`;

  const crfInput = document.getElementById('modalCrf');
  const crfLabel = document.getElementById('modalCrfVal');
  const crfTitleEl = document.getElementById('modalCrfTitle');
  const modalPresetGroup = document.getElementById('modalPresetGroup');
  const presetEl = document.getElementById('modalPreset');

  if (crfInput) crfInput.value = crf;
  if (crfLabel) crfLabel.textContent = isApple ? `Q${qVal}` : crf;

  if (isApple) {
    if (crfTitleEl) crfTitleEl.innerHTML = `Apple 硬件质量系数: <strong id="modalCrfVal">Q${qVal}</strong> <span class="hint" id="modalCrfHint">(底层 -q:v，Q越高清越大，推荐 Q50-Q70)</span>`;
    if (modalPresetGroup) modalPresetGroup.style.display = 'none';
  } else {
    if (crfTitleEl) crfTitleEl.innerHTML = `CRF 质量因子: <strong id="modalCrfVal">${crf}</strong> <span class="hint" id="modalCrfHint">(值越大文件越小，18-32)</span>`;
    if (modalPresetGroup) modalPresetGroup.style.display = 'block';
    if (presetEl) presetEl.disabled = false;
  }

  if (presetEl) presetEl.value = params.preset || 'fast';

  const audioEl = document.getElementById('modalAudioBitrate');
  if (audioEl) audioEl.value = params.audio_bitrate || '128k';

  const keyEl = document.getElementById('modalKeyframe');
  if (keyEl) keyEl.value = params.keyframe_interval || 2;

  const subEl = document.getElementById('modalSubtitlePolicy');
  if (subEl) subEl.value = params.subtitle_policy || 'copy_all';

  const fastEl = document.getElementById('modalFastStart');
  if (fastEl) fastEl.checked = params.faststart !== false;

  const maxFpsEl = document.getElementById('modalMaxFPS');
  if (maxFpsEl) maxFpsEl.value = String(params.max_fps !== undefined ? params.max_fps : 30);

  refreshModalSavings();

  const modal = document.getElementById('presetEditModal');
  if (modal) modal.style.display = 'flex';
}

export function closePresetEditModal() {
  const modal = document.getElementById('presetEditModal');
  if (modal) modal.style.display = 'none';
  currentEditingProfile = null;
}

export function initPresetEditModal() {
  const modal = document.getElementById('presetEditModal');
  const closeBtn = document.getElementById('closePresetModalBtn');
  const cancelBtn = document.getElementById('cancelPresetModalBtn');
  const saveBtn = document.getElementById('savePresetModalBtn');
  const crfInput = document.getElementById('modalCrf');

  if (crfInput) {
    crfInput.addEventListener('input', (e) => {
      const val = parseInt(e.target.value, 10);
      const curCodec = getSelectedBaseCodec();
      const curApple = curCodec === 'hevc_videotoolbox' || curCodec === 'h264_videotoolbox';
      const curQ = Math.max(20, Math.min(95, 50 - (val - 24) * 2));
      const crfLabel = document.getElementById('modalCrfVal');
      if (crfLabel) crfLabel.textContent = curApple ? `Q${curQ}` : val;
      refreshModalSavings();
    });
  }

  ['modalPreset', 'modalAudioBitrate'].forEach((id) => {
    const el = document.getElementById(id);
    if (el) el.addEventListener('change', refreshModalSavings);
  });

  if (modal) {
    modal.addEventListener('click', (e) => {
      if (e.target === modal) {
        closePresetEditModal();
      }
    });
  }

  if (closeBtn) closeBtn.addEventListener('click', closePresetEditModal);
  if (cancelBtn) cancelBtn.addEventListener('click', closePresetEditModal);

  if (saveBtn) {
    saveBtn.addEventListener('click', async () => {
      if (!currentEditingProfile) return;
      const currentCodec = getSelectedBaseCodec();
      const baseParams = currentEditingProfile.effective_params || currentEditingProfile.default_params;
      const subEl = document.getElementById('modalSubtitlePolicy');
      const subtitlePolicy = subEl ? subEl.value : 'copy_all';
      const maxFpsEl = document.getElementById('modalMaxFPS');
      const maxFps = maxFpsEl ? parseInt(maxFpsEl.value, 10) : 30;

      const updatedParams = {
        ...baseParams,
        video_codec: currentCodec,
        crf: parseInt(document.getElementById('modalCrf').value),
        preset: document.getElementById('modalPreset').value,
        audio_bitrate: document.getElementById('modalAudioBitrate').value,
        keyframe_interval: parseInt(document.getElementById('modalKeyframe').value),
        subtitle_policy: subtitlePolicy,
        faststart: document.getElementById('modalFastStart').checked,
        max_fps: isNaN(maxFps) ? 30 : maxFps,
      };

      await saveCustomProfile(currentEditingProfile.name, updatedParams);
      closePresetEditModal();
    });
  }
}

export async function saveCustomProfile(name, params) {
  try {
    const res = await apiFetch(`/api/v1/profiles/${name}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });
    if (res.ok) {
      showToast(`已成功将自定预设保存到档位 [${name}]，已持久化到数据库！`, 'success');
      await loadProfiles();
    } else {
      showAlert({ title: '保存失败', message: '保存预设失败，请检查参数合法性或操作权限', type: 'error' });
    }
  } catch (e) {
    showAlert({ title: '网络请求错误', message: '保存预设失败: ' + e, type: 'error' });
  }
}

export async function resetProfile(name) {
  try {
    const res = await apiFetch(`/api/v1/profiles/${name}/reset`, { method: 'POST' });
    if (res.ok) {
      await loadProfiles();
    } else {
      showAlert({ title: '重置失败', message: '重置预设失败', type: 'error' });
    }
  } catch (e) {
    showAlert({ title: '网络请求错误', message: '重置预设失败: ' + e, type: 'error' });
  }
}

export function configureHardwareCodec(accels) {
  const container = document.getElementById('hwCodecContainer');
  if (!container) return;

  try {
    if (accels && accels.length > 0) {
      localStorage.setItem('slimbox_hw_accels', JSON.stringify(accels));
    } else {
      localStorage.removeItem('slimbox_hw_accels');
      localStorage.removeItem('slimbox_selected_codec');
    }
  } catch (_) {}

  container.innerHTML = '';
  if (!accels || accels.length === 0) {
    const defaultRadio = document.getElementById('codecH265');
    if (defaultRadio) defaultRadio.checked = true;
    renderProfiles();
    return;
  }

  const savedCodec = (() => {
    try { return localStorage.getItem('slimbox_selected_codec'); } catch (_) { return null; }
  })();
  const currentlyChecked = document.querySelector('input[name="baseCodec"]:checked')?.value || savedCodec;
  let hasChecked = false;

  const hwList = [];
  const codecMap = {
    amf: [{ codec: 'hevc_amf', label: 'AMD H.265' }, { codec: 'h264_amf', label: 'AMD H.264' }],
    nvenc: [{ codec: 'hevc_nvenc', label: 'NVIDIA H.265' }, { codec: 'h264_nvenc', label: 'NVIDIA H.264' }],
    qsv: [{ codec: 'hevc_qsv', label: 'Intel H.265' }, { codec: 'h264_qsv', label: 'Intel H.264' }],
    videotoolbox: [{ codec: 'hevc_videotoolbox', label: 'Apple H.265' }, { codec: 'h264_videotoolbox', label: 'Apple H.264' }],
    vaapi_intel: [{ codec: 'hevc_vaapi', label: 'Intel H.265' }, { codec: 'h264_vaapi', label: 'Intel H.264' }],
    vaapi_amd: [{ codec: 'hevc_vaapi', label: 'AMD H.265' }, { codec: 'h264_vaapi', label: 'AMD H.264' }],
    vaapi: [{ codec: 'hevc_vaapi', label: 'Intel H.265' }, { codec: 'h264_vaapi', label: 'Intel H.264' }],
  };
  if (accels && accels.length > 0) {
    accels.forEach((a) => {
      (codecMap[a] || []).forEach((c) => hwList.push({ ...c, tag: '⚡ 硬编' }));
    });
  }

  container.innerHTML = '';
  hwList.forEach((hw, idx) => {
    const isSelected = currentlyChecked ? (currentlyChecked === hw.codec) : (idx === 0);
    if (isSelected) hasChecked = true;

    const label = document.createElement('label');
    label.className = 'radio-label';
    label.innerHTML = `<input type="radio" name="baseCodec" value="${hw.codec}" ${isSelected ? 'checked' : ''}><span class="radio-custom"></span><strong>${hw.label}</strong><span class="tag-speed" style="background: rgba(16, 185, 129, 0.2); color: #34d399; border: 1px solid rgba(16, 185, 129, 0.4); white-space: nowrap;">${hw.tag}</span>`;
    container.appendChild(label);
  });

  if (hwList.length > 0 && !hasChecked) {
    const firstRadio = container.querySelector('input[name="baseCodec"]');
    if (firstRadio) {
      firstRadio.checked = true;
      hasChecked = true;
    }
  }

  if (!hasChecked) {
    const h265Radio = document.getElementById('codecH265');
    if (h265Radio) h265Radio.checked = true;
  }

  if (advSelect && accels && accels.length > 0) {
    Array.from(advSelect.options).forEach((opt) => {
      const isHW = accels.some((a) => (a.startsWith('vaapi') ? opt.value.includes('vaapi') : opt.value.includes(a)));
      if (isHW) {
        if (accels.includes('vaapi_intel') && opt.value.includes('vaapi')) opt.textContent = opt.textContent.replace('VAAPI 硬件加速', 'Intel 硬件加速');
        else if (accels.includes('vaapi_amd') && opt.value.includes('vaapi')) opt.textContent = opt.textContent.replace('VAAPI 硬件加速', 'AMD 硬件加速');
        if (!opt.textContent.startsWith('⚡')) opt.textContent = `⚡ ${opt.textContent}`;
      }
    });
  }

  renderProfiles();
}

export async function initProfiles() {
  const quickToggle = document.getElementById('codecQuickToggle');
  if (quickToggle) {
    quickToggle.addEventListener('change', (e) => {
      if (e.target && e.target.name === 'baseCodec') {
        try { localStorage.setItem('slimbox_selected_codec', e.target.value); } catch (_) {}
        renderProfiles();
      }
    });
  }

  // Pre-hydrate hardware encoders immediately from cache before any network calls
  try {
    const cached = JSON.parse(localStorage.getItem('slimbox_hw_accels') || 'null');
    if (cached && Array.isArray(cached) && cached.length > 0) {
      configureHardwareCodec(cached);
    }
  } catch (_) {}

  await loadProfiles();
  initPresetEditModal();

  bus.on('auth:unlocked', () => loadProfiles());
  bus.on('system:stats', (d) => {
    if (d && d.hardware_accelerators !== undefined) configureHardwareCodec(d.hardware_accelerators);
  });
  bus.on('gpu:updated', (cfg) => {
    if (!cfg || cfg.preferred_gpu === 'cpu_only') {
      configureHardwareCodec([]);
    } else if (cfg.active_gpu) {
      const v = cfg.active_gpu.vendor;
      const t = cfg.active_gpu.type;
      if (v === 'intel' && t === 'vaapi') configureHardwareCodec(['vaapi_intel']);
      else if (v === 'amd' && t === 'vaapi') configureHardwareCodec(['vaapi_amd']);
      else if (t) configureHardwareCodec([t]);
    }
  });
}
