export class TaskPageWheel {
  private last = -Infinity;
  private total = 0;
  private consumed = false;
  private lockedUntil = 0;

  step(delta: number, now: number, transition: number): -1 | 0 | 1 {
    if (!Number.isFinite(delta) || delta === 0) return 0;
    if (now - this.last > 180) {
      this.total = 0;
      this.consumed = false;
    }
    this.last = now;
    if (this.consumed) return 0;
    if (now < this.lockedUntil) {
      this.consumed = true;
      return 0;
    }
    if (Math.sign(this.total) !== Math.sign(delta)) this.total = 0;
    this.total += delta;
    if (Math.abs(this.total) < 64) return 0;
    this.consumed = true;
    this.lockedUntil = now + transition;
    return this.total > 0 ? 1 : -1;
  }
}
