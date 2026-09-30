import { bus } from '../bus.js';

export const THEME_PRESETS = [
  { id: 'nordic-cool', name: '❄️ 极简冷灰 (默认)', color: '#64748b', tag: '雅致冷灰 / 深蓝灰' },
  { id: 'light-warm', name: '☀️ 温暖浅沙', color: '#f59e0b', tag: '米沙暖白 / 暖阳橙' },
  { id: 'macaron-blue', name: '🌊 马卡龙蓝', color: '#38bdf8', tag: '天青淡蓝 / 蔚蓝水' },
  { id: 'macaron-green', name: '🍃 马卡龙绿', color: '#4ade80', tag: '清爽薄荷 / 嫩草绿' },
  { id: 'macaron-yellow', name: '🍌 马卡龙黄', color: '#facc15', tag: '活力柠檬 / 浅鹅黄' },
  { id: 'macaron-pink', name: '🍭 马卡龙粉', color: '#f472b6', tag: '甜美糖粉 / 樱花粉' },
  { id: 'sakura-peach', name: '🌸 樱粉蜜桃', color: '#ec4899', tag: '柔粉浅白 / 蜜桃粉' },
  { id: 'autumn-maple', name: '🍁 枫叶秋枫', color: '#ea580c', tag: '暖秋枫黄 / 枫叶橙' },
  { id: 'macaron-purple', name: '🍇 马卡龙紫', color: '#c084fc', tag: '优雅淡紫 / 浅葡萄' },
  { id: 'macaron-orange', name: '🍊 马卡龙橙', color: '#fb923c', tag: '鲜甜暖橙 / 蜜橘橙' },
  { id: 'dark-neon', name: '🌌 暗色霓虹', color: '#8b5cf6', tag: '深蓝暗夜 / 霓虹紫' },
  { id: 'dark-cyber', name: '🦾 暗色赛博', color: '#00f0ff', tag: '赛博夜黑 / 电光青' },
  { id: 'dark-obsidian', name: '🖤 黑曜石暗', color: '#1e293b', tag: '深曜石黑 / 雅致金' },
  { id: 'deep-forest', name: '🌲 深邃苍林', color: '#10b981', tag: '幽深墨绿 / 荧光翠' },
  { id: 'aurora-night', name: '💚 极光幻夜', color: '#06b6d4', tag: '玄夜深蓝 / 极光青' },
  { id: 'violet-dream', name: '💜 罗兰紫幻', color: '#a855f7', tag: '梦幻深紫 / 荧光紫' }
];

export function getCurrentTheme() {
  try {
    return localStorage.getItem('slimbox_theme') || 'nordic-cool';
  } catch (e) {
    return 'nordic-cool';
  }
}

export function applyTheme(themeId) {
  if (!themeId) themeId = 'nordic-cool';
  try {
    localStorage.setItem('slimbox_theme', themeId);
  } catch (e) {}
  document.documentElement.className = 'theme-' + themeId;
  updateThemeUI(themeId);
  bus.emit('theme:changed', themeId);
}

export function updateThemeUI(themeId) {
  const currentPreset = THEME_PRESETS.find(t => t.id === themeId) || THEME_PRESETS[0];
  const descEl = document.getElementById('currentThemeDesc');
  if (descEl) {
    descEl.textContent = `当前：${currentPreset.name}`;
  }

  const cards = document.querySelectorAll('.theme-card');
  cards.forEach(card => {
    if (card.dataset.themeId === themeId) {
      card.classList.add('active');
    } else {
      card.classList.remove('active');
    }
  });
}

export function renderThemeGrid() {
  const container = document.getElementById('themeGrid');
  if (!container) return;
  container.innerHTML = '';

  const activeTheme = getCurrentTheme();

  THEME_PRESETS.forEach(preset => {
    const card = document.createElement('div');
    card.className = 'theme-card' + (preset.id === activeTheme ? ' active' : '');
    card.dataset.themeId = preset.id;
    card.innerHTML = `
      <div class="theme-swatch" style="background: ${preset.color};"></div>
      <div class="theme-meta">
        <span class="theme-name">${preset.name}</span>
        <span class="theme-tag">${preset.tag}</span>
      </div>
    `;

    card.addEventListener('click', () => {
      applyTheme(preset.id);
    });

    container.appendChild(card);
  });
}

export function initTheme() {
  applyTheme(getCurrentTheme());
  renderThemeGrid();
}
