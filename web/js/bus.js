// Lightweight event bus for cross-module decoupled communication
const listeners = new Map();

export const bus = {
  /**
   * Subscribe to an event.
   * @param {string} event
   * @param {Function} callback
   * @returns {Function} Unsubscribe function
   */
  on(event, callback) {
    if (!listeners.has(event)) {
      listeners.set(event, new Set());
    }
    listeners.get(event).add(callback);
    return () => this.off(event, callback);
  },

  /**
   * Unsubscribe from an event.
   * @param {string} event
   * @param {Function} callback
   */
  off(event, callback) {
    const set = listeners.get(event);
    if (set) {
      set.delete(callback);
      if (set.size === 0) {
        listeners.delete(event);
      }
    }
  },

  /**
   * Emit an event with optional payload data.
   * @param {string} event
   * @param {any} [data]
   */
  emit(event, data) {
    const set = listeners.get(event);
    if (set) {
      Array.from(set).forEach((cb) => {
        try {
          cb(data);
        } catch (err) {
          console.error(`[EventBus] Error in listener for event "${event}":`, err);
        }
      });
    }
  }
};
