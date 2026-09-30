// SlimBox Dashboard Main Orchestrator (ES Module Entrypoint)

import { initAuth } from './modules/auth.js';
import { initStats } from './modules/stats.js';
import { initProfiles } from './modules/profiles.js';
import { initUpload } from './modules/upload.js';
import { initTasksPolling } from './modules/tasks.js';
import { initSettingsModal } from './modules/settings.js';
import { initTheme } from './modules/theme.js';
import { initDialogSystem } from './modules/dialog.js';
import { initCliGuide } from './modules/guide.js';

document.addEventListener('DOMContentLoaded', async () => {
  // 1. Initialize modern custom dialogs and alert interception
  initDialogSystem();

  // 2. Apply theme immediately to prevent layout / styling flash
  initTheme();

  // 3. Initialize authentication & verify current token/status
  await initAuth();

  // 4. Initialize functional dashboard modules
  initStats();
  await initProfiles();
  initUpload();
  initTasksPolling();
  initSettingsModal();
  initCliGuide();
});
