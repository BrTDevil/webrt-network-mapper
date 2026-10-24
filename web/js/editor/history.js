// Undo / redo pe comenzi. Fiecare comandă ține starea scenei înainte și după
// (serializată), deci orice operație devine reversibilă fără cod special.

export class History {
  constructor(limit = 200) {
    this.limit = limit;
    this.undoStack = [];
    this.redoStack = [];
    this.onChange = () => {};
  }

  push(label, before, after) {
    if (before === after) return;
    this.undoStack.push({ label, before, after });
    if (this.undoStack.length > this.limit) this.undoStack.shift();
    this.redoStack = [];
    this.onChange();
  }

  undo() {
    const c = this.undoStack.pop();
    if (!c) return null;
    this.redoStack.push(c);
    this.onChange();
    return c.before;
  }

  redo() {
    const c = this.redoStack.pop();
    if (!c) return null;
    this.undoStack.push(c);
    this.onChange();
    return c.after;
  }

  clear() {
    this.undoStack = [];
    this.redoStack = [];
    this.onChange();
  }

  get canUndo() { return this.undoStack.length > 0; }
  get canRedo() { return this.redoStack.length > 0; }
  get undoLabel() { return this.undoStack.at(-1)?.label || ''; }
  get redoLabel() { return this.redoStack.at(-1)?.label || ''; }
}
