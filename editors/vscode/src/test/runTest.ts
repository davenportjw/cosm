/**
 * Standalone test runner for Cosm VS Code Extension test suite.
 */

import * as path from 'path';
import * as fs from 'fs';

// 1. Mock 'vscode' module in Node resolution
const Module = require('module');
const origResolve = Module._resolveFilename;
Module._resolveFilename = function (request: string, parent: any, isMain: boolean, options: any) {
  if (request === 'vscode') {
    return 'vscode';
  }
  return origResolve.call(this, request, parent, isMain, options);
};

class MockTreeItem {
  label?: any;
  collapsibleState?: any;
  description?: any;
  tooltip?: any;
  iconPath?: any;
  contextValue?: any;
  command?: any;
  resourceUri?: any;
  constructor(labelOrUri: any, collapsibleState?: any) {
    if (typeof labelOrUri === 'string' || (labelOrUri && labelOrUri.label)) {
      this.label = labelOrUri;
    } else {
      this.resourceUri = labelOrUri;
    }
    this.collapsibleState = collapsibleState;
  }
}

class MockThemeIcon {
  static readonly File = new MockThemeIcon('file');
  static readonly Folder = new MockThemeIcon('folder');
  id: string;
  constructor(id: string) {
    this.id = id;
  }
}

class MockEventEmitter<T = any> {
  private listeners: ((e: T) => any)[] = [];
  event = (listener: (e: T) => any) => {
    this.listeners.push(listener);
    return {
      dispose: () => {
        const idx = this.listeners.indexOf(listener);
        if (idx >= 0) this.listeners.splice(idx, 1);
      }
    };
  };
  fire(data: T) {
    for (const l of this.listeners) {
      l(data);
    }
  }
  dispose() {
    this.listeners = [];
  }
}

class MockMarkdownString {
  value: string;
  isTrusted?: boolean;
  supportThemeIcons?: boolean;
  constructor(val?: string) {
    this.value = val || '';
  }
  appendMarkdown(v: string) {
    this.value += v;
  }
  appendCodeblock(code: string, lang?: string) {
    this.value += `\n\`\`\`${lang || ''}\n${code}\n\`\`\`\n`;
  }
}

const vscodeMock = {
  workspace: {
    workspaceFolders: undefined,
    getConfiguration: () => ({
      get: (key: string, defaultValue: any) => defaultValue
    }),
    findFiles: async () => [],
    openTextDocument: async () => ({})
  },
  window: {
    createStatusBarItem: () => ({
      show: () => {},
      dispose: () => {},
      text: '',
      tooltip: ''
    }),
    showInformationMessage: async () => undefined,
    showErrorMessage: async () => undefined,
    showWarningMessage: async () => undefined
  },
  Uri: {
    file: (p: string) => ({ fsPath: p, scheme: 'file' }),
    parse: (s: string) => ({ fsPath: s, scheme: 'cosm' })
  },
  StatusBarAlignment: { Left: 1, Right: 2 },
  ViewColumn: { One: 1, Beside: -2 },
  TreeItem: MockTreeItem,
  ThemeIcon: MockThemeIcon,
  MarkdownString: MockMarkdownString,
  TreeItemCollapsibleState: { None: 0, Collapsed: 1, Expanded: 2 },
  EventEmitter: MockEventEmitter
};

require.cache['vscode'] = {
  id: 'vscode',
  filename: 'vscode',
  loaded: true,
  exports: vscodeMock
} as any;

// 2. Minimal test harness
const testSuite: { name: string; fn: () => void | Promise<void> }[] = [];
let currentSuite = '';

(globalThis as any).describe = (name: string, fn: () => void) => {
  const previousSuite = currentSuite;
  currentSuite = previousSuite ? `${previousSuite} > ${name}` : name;
  fn();
  currentSuite = previousSuite;
};

(globalThis as any).it = (name: string, fn: () => void | Promise<void>) => {
  testSuite.push({ name: `${currentSuite} > ${name}`, fn });
};

export async function run() {
  console.log('🌌 Running Cosm VS Code Extension Test Suite...\n');

  // Dynamically import the test file
  await import('./extension.test');

  let passed = 0;
  let failed = 0;
  const start = Date.now();

  for (const t of testSuite) {
    try {
      await t.fn();
      console.log(`  ✓ ${t.name}`);
      passed++;
    } catch (err: any) {
      console.error(`  ✗ ${t.name}`);
      console.error(`    ${err.stack || err.message}`);
      failed++;
    }
  }

  const elapsed = Date.now() - start;
  console.log(`\n======================================================`);
  console.log(`Total: ${testSuite.length} | Passed: ${passed} | Failed: ${failed} | Time: ${elapsed}ms`);
  console.log(`======================================================\n`);

  if (failed > 0) {
    process.exit(1);
  }
}

run().catch(err => {
  console.error('Fatal test runner error:', err);
  process.exit(1);
});
