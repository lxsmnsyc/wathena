import { SkillTarget } from '../gen/wathena/v1/event_pb';
import type { UseSkillTarget } from '../net/connection';

// Picking a skill's target, as RO does. A self skill casts at once. An
// enemy or support skill waits for a click on an entity, and a support
// skill may also target the player itself. A ground or trap skill waits
// for a click on a cell. Escape or a right click cancels.

/** A UseSkill command to send. */
export interface CastRequest {
  skillId: number;
  level: number;
  target: UseSkillTarget;
}

/** What a waiting cast needs: an entity or a cell. */
export type TargetKind = 'entity' | 'cell';

interface Waiting {
  skillId: number;
  level: number;
  kind: TargetKind;
  /** Whether the player itself is a valid target. */
  self: boolean;
}

/** The kind of target a skill needs, or 'self', or undefined when it cannot be cast. */
export function targetKind(target: SkillTarget): TargetKind | 'self' | undefined {
  switch (target) {
    case SkillTarget.SELF:
      return 'self';
    case SkillTarget.ENEMY:
    case SkillTarget.SUPPORT:
      return 'entity';
    case SkillTarget.GROUND:
    case SkillTarget.TRAP:
      return 'cell';
    default:
      return undefined;
  }
}

/** The target mode: the skill waiting for a click, if any. */
export class TargetMode {
  private waiting: Waiting | undefined;

  /** The kind of click the waiting skill needs, or undefined when none waits. */
  get kind(): TargetKind | undefined {
    return this.waiting?.kind;
  }

  /** The skill that waits for a target, if any. */
  get skillId(): number | undefined {
    return this.waiting?.skillId;
  }

  /**
   * Starts casting a skill at a level. A self skill returns its request
   * at once. Other skills wait for a click and return undefined. A
   * passive skill does nothing. A new skill replaces the one waiting.
   */
  begin(skillId: number, level: number, target: SkillTarget): CastRequest | undefined {
    const kind = targetKind(target);
    if (kind === undefined) {
      return undefined;
    }
    if (kind === 'self') {
      this.waiting = undefined;
      return { skillId, level, target: { case: undefined } };
    }
    this.waiting = { skillId, level, kind, self: target === SkillTarget.SUPPORT };
    return undefined;
  }

  /**
   * A click on an entity. self is whether it is the player. It returns
   * the request and ends the mode when the waiting skill takes it.
   */
  clickEntity(id: number, self: boolean): CastRequest | undefined {
    const w = this.waiting;
    if (w?.kind !== 'entity' || (self && !w.self)) {
      return undefined;
    }
    this.waiting = undefined;
    return { skillId: w.skillId, level: w.level, target: { case: 'entity', value: id } };
  }

  /** A click on a cell. It returns the request and ends the mode for a ground skill. */
  clickCell(x: number, y: number): CastRequest | undefined {
    const w = this.waiting;
    if (w?.kind !== 'cell') {
      return undefined;
    }
    this.waiting = undefined;
    return { skillId: w.skillId, level: w.level, target: { case: 'cell', value: { x, y } } };
  }

  /** Stops waiting. Returns whether a skill was waiting. */
  cancel(): boolean {
    const was = this.waiting !== undefined;
    this.waiting = undefined;
    return was;
  }
}
