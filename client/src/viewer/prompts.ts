// Small prompts that ask the player to accept or decline, such as a
// party invite or a friend request. A new prompt with the same key
// replaces the old one. User text is only ever set with textContent.

/** How long a prompt waits before it declines by itself, in milliseconds. */
export const PROMPT_MS = 30_000;

/** A prompt to show. */
export interface Prompt {
  /** Prompts with the same key replace each other, such as party:12. */
  key: string;
  text: string;
  accept: () => void;
  decline: () => void;
}

/**
 * The prompts on the page, in div#social-prompts. Each is a
 * div.social-prompt[data-prompt] with button.prompt-accept and
 * button.prompt-decline.
 */
export class Prompts {
  private readonly shown = new Map<
    string,
    { div: HTMLDivElement; timer: ReturnType<typeof setTimeout> }
  >();

  constructor(private readonly root: HTMLElement) {}

  /** Shows a prompt. It declines by itself after PROMPT_MS. */
  show(p: Prompt): void {
    this.close(p.key);
    const div = document.createElement('div');
    div.className = 'social-prompt';
    div.dataset.prompt = p.key;
    div.setAttribute('role', 'alertdialog');
    const text = document.createElement('p');
    text.textContent = p.text;
    const accept = document.createElement('button');
    accept.type = 'button';
    accept.className = 'prompt-accept';
    accept.textContent = 'Accept';
    const decline = document.createElement('button');
    decline.type = 'button';
    decline.className = 'prompt-decline';
    decline.textContent = 'Decline';
    accept.addEventListener('click', () => {
      this.close(p.key);
      p.accept();
    });
    decline.addEventListener('click', () => {
      this.close(p.key);
      p.decline();
    });
    div.append(text, accept, ' ', decline);
    const timer = setTimeout(() => {
      if (this.shown.get(p.key)?.div === div) {
        this.close(p.key);
        p.decline();
      }
    }, PROMPT_MS);
    this.shown.set(p.key, { div, timer });
    this.root.append(div);
    this.root.hidden = false;
  }

  /** Removes a prompt without an answer. */
  close(key: string): void {
    const s = this.shown.get(key);
    if (s === undefined) {
      return;
    }
    clearTimeout(s.timer);
    s.div.remove();
    this.shown.delete(key);
    this.root.hidden = this.shown.size === 0;
  }
}
