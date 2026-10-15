// Title bars that move windows, with a mouse, a pen or a finger (review
// decision 39). Below 600 pixels wide, windows are full-width panels
// (viewer.css), so a bar moves its window up and down only.

/** The query for screens that show windows as full-width panels. */
export const SMALL_SCREEN = '(width < 600px)';

let front = 100;

/** Adds a title bar to a window, which moves it and brings it to the front. */
export function makeMovable(win: HTMLElement, title: string): void {
  const bar = document.createElement('div');
  bar.className = 'window-bar';
  const label = document.createElement('span');
  label.textContent = title;
  bar.append(label);
  win.prepend(bar);
  win.classList.add('game-window');
  win.addEventListener(
    'pointerdown',
    () => {
      front++;
      win.style.zIndex = String(front);
    },
    { capture: true },
  );
  bar.addEventListener('pointerdown', (e) => {
    if (e.pointerType === 'mouse' && e.button !== 0) {
      return;
    }
    e.preventDefault();
    const rect = win.getBoundingClientRect();
    const offsetX = e.clientX - rect.left;
    const offsetY = e.clientY - rect.top;
    const narrow = matchMedia(SMALL_SCREEN).matches;
    bar.setPointerCapture(e.pointerId);
    const move = (m: PointerEvent): void => {
      const top = Math.min(Math.max(m.clientY - offsetY, 0), innerHeight - bar.offsetHeight);
      if (narrow) {
        // The panel rules of viewer.css read these.
        win.style.setProperty('--panel-top', `${top}px`);
        win.style.setProperty('--panel-bottom', 'auto');
        return;
      }
      const left = Math.min(Math.max(m.clientX - offsetX, 0), innerWidth - 48);
      win.style.top = `${top}px`;
      win.style.bottom = 'auto';
      win.style.left = `${left}px`;
      win.style.right = 'auto';
      win.style.transform = 'none';
    };
    const up = (): void => {
      bar.removeEventListener('pointermove', move);
      bar.removeEventListener('pointerup', up);
      bar.removeEventListener('pointercancel', up);
    };
    bar.addEventListener('pointermove', move);
    bar.addEventListener('pointerup', up);
    bar.addEventListener('pointercancel', up);
  });
}
