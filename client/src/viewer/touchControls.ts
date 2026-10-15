// The on-screen controls of touch (review decision 39): the joystick in
// the lower left corner, a chat button, and a cancel button while a skill
// waits for its target. They show while the page's body has the class
// `touch` (touch.ts). The larger hotkey bar and the window buttons are
// the usual ones, styled for touch in viewer.css.

import { type Push, joystickPush, knobOffset } from './joystick';

export interface TouchControlActions {
  /** The joystick's push changed, or ended with undefined. */
  push: (push: Push | undefined) => void;
  /** The joystick was let go. */
  release: () => void;
  chat: () => void;
  cancelTarget: () => void;
}

export class TouchControls {
  readonly root: HTMLDivElement;
  readonly joystick: HTMLDivElement;
  private readonly knob: HTMLDivElement;
  private readonly cancel: HTMLButtonElement;
  private pointer: number | undefined;

  constructor(parent: HTMLElement, actions: TouchControlActions) {
    this.root = document.createElement('div');
    this.root.id = 'touch-controls';
    this.joystick = document.createElement('div');
    this.joystick.id = 'joystick';
    this.joystick.setAttribute('role', 'application');
    this.joystick.setAttribute('aria-label', 'Joystick: push to walk');
    this.knob = document.createElement('div');
    this.knob.className = 'joystick-knob';
    this.joystick.append(this.knob);

    const chat = document.createElement('button');
    chat.type = 'button';
    chat.id = 'chat-button';
    chat.textContent = 'Chat';
    chat.addEventListener('click', () => {
      actions.chat();
    });

    this.cancel = document.createElement('button');
    this.cancel.type = 'button';
    this.cancel.id = 'target-cancel';
    this.cancel.textContent = 'Cancel skill';
    this.cancel.hidden = true;
    this.cancel.addEventListener('click', () => {
      actions.cancelTarget();
    });

    this.root.append(this.joystick, chat, this.cancel);
    parent.append(this.root);

    const center = (): [number, number, number] => {
      const r = this.joystick.getBoundingClientRect();
      return [r.left + r.width / 2, r.top + r.height / 2, r.width / 2];
    };
    const track = (e: PointerEvent): void => {
      const [cx, cy, radius] = center();
      const [kx, ky] = knobOffset(cx, cy, radius, e.clientX, e.clientY);
      this.knob.style.transform = `translate(${kx}px, ${ky}px)`;
      actions.push(joystickPush(cx, cy, radius, e.clientX, e.clientY));
    };
    const stop = (e: PointerEvent): void => {
      if (e.pointerId !== this.pointer) {
        return;
      }
      this.pointer = undefined;
      this.knob.style.transform = '';
      this.joystick.classList.remove('active');
      actions.push(undefined);
      actions.release();
    };
    this.joystick.addEventListener('pointerdown', (e) => {
      if (this.pointer !== undefined) {
        return;
      }
      e.preventDefault();
      this.pointer = e.pointerId;
      this.joystick.setPointerCapture(e.pointerId);
      this.joystick.classList.add('active');
      track(e);
    });
    this.joystick.addEventListener('pointermove', (e) => {
      if (e.pointerId === this.pointer) {
        track(e);
      }
    });
    this.joystick.addEventListener('pointerup', stop);
    this.joystick.addEventListener('pointercancel', stop);
  }

  /** Whether the joystick shows, as the setting says. */
  get joystickShown(): boolean {
    return !this.joystick.hidden;
  }

  set joystickShown(shown: boolean) {
    this.joystick.hidden = !shown;
  }

  /** Whether the cancel button shows, while a skill waits for its target. */
  get targeting(): boolean {
    return !this.cancel.hidden;
  }

  set targeting(on: boolean) {
    if (this.cancel.hidden === on) {
      this.cancel.hidden = !on;
    }
  }
}
