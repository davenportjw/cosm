declare module 'vscode' {
  export class Uri {
    readonly scheme: string;
    readonly authority: string;
    readonly path: string;
    readonly query: string;
    readonly fragment: string;
    readonly fsPath: string;
    static file(path: string): Uri;
    static parse(value: string, strict?: boolean): Uri;
    toString(): string;
  }

  export interface Disposable {
    dispose(): any;
  }

  export class EventEmitter<T> {
    readonly event: Event<T>;
    fire(data: T): void;
    dispose(): void;
  }

  export interface Event<T> {
    (listener: (e: T) => any, thisArgs?: any, disposables?: Disposable[]): Disposable;
  }

  export interface CancellationToken {
    isCancellationRequested: boolean;
  }

  export class Position {
    readonly line: number;
    readonly character: number;
    constructor(line: number, character: number);
  }

  export class Range {
    readonly start: Position;
    readonly end: Position;
    constructor(start: Position, end: Position);
    constructor(startLine: number, startCharacter: number, endLine: number, endCharacter: number);
  }

  export class Selection extends Range {
    readonly anchor: Position;
    readonly active: Position;
    constructor(anchor: Position, active: Position);
  }

  export enum TextEditorRevealType {
    Default = 0,
    InCenter = 1,
    InCenterIfOutsideViewport = 2,
    AtTop = 3
  }

  export interface TextDocument {
    readonly uri: Uri;
    readonly fileName: string;
    readonly isUntitled: boolean;
    readonly languageId: string;
    readonly version: number;
    readonly lineCount: number;
    getText(range?: Range): string;
    lineAt(line: number): TextLine;
    positionAt(offset: number): Position;
    getWordRangeAtPosition(position: Position, regex?: RegExp): Range | undefined;
  }

  export interface TextLine {
    readonly lineNumber: number;
    readonly text: string;
    readonly range: Range;
  }

  export interface TextEditor {
    readonly document: TextDocument;
    selection: Selection;
    revealRange(range: Range, revealType?: TextEditorRevealType): void;
  }

  export class MarkdownString {
    value: string;
    isTrusted?: boolean;
    supportHtml?: boolean;
    constructor(value?: string);
    appendMarkdown(value: string): MarkdownString;
    appendText(value: string): MarkdownString;
  }

  export interface Command {
    title: string;
    command: string;
    tooltip?: string;
    arguments?: any[];
  }

  export class CodeLens {
    range: Range;
    command?: Command;
    readonly isResolved: boolean;
    constructor(range: Range, command?: Command);
  }

  export interface CodeLensProvider {
    onDidChangeCodeLenses?: Event<void>;
    provideCodeLenses(document: TextDocument, token: CancellationToken): ProviderResult<CodeLens[]>;
    resolveCodeLens?(codeLens: CodeLens, token: CancellationToken): ProviderResult<CodeLens>;
  }

  export class Hover {
    readonly contents: MarkdownString[] | MarkdownString;
    readonly range?: Range;
    constructor(contents: MarkdownString | MarkdownString[], range?: Range);
  }

  export interface HoverProvider {
    provideHover(document: TextDocument, position: Position, token: CancellationToken): ProviderResult<Hover>;
  }

  export enum TreeItemCollapsibleState {
    None = 0,
    Collapsed = 1,
    Expanded = 2
  }

  export class ThemeColor {
    readonly id: string;
    constructor(id: string);
  }

  export class ThemeIcon {
    static readonly File: ThemeIcon;
    static readonly Folder: ThemeIcon;
    readonly id: string;
    readonly color?: ThemeColor;
    constructor(id: string, color?: ThemeColor);
  }

  export interface TreeItemLabel {
    label: string;
    highlights?: [number, number][];
  }

  export class TreeItem {
    label?: string | TreeItemLabel;
    id?: string;
    iconPath?: string | Uri | { light: string | Uri; dark: string | Uri } | ThemeIcon;
    description?: string | boolean;
    resourceUri?: Uri;
    tooltip?: string | MarkdownString;
    command?: Command;
    collapsibleState?: TreeItemCollapsibleState;
    contextValue?: string;
    accessibilityInformation?: any;
    constructor(label: string | TreeItemLabel, collapsibleState?: TreeItemCollapsibleState);
    constructor(resourceUri: Uri, collapsibleState?: TreeItemCollapsibleState);
  }

  export interface TreeDataProvider<T> {
    onDidChangeTreeData?: Event<T | undefined | null | void>;
    getTreeItem(element: T): TreeItem | Thenable<TreeItem>;
    getChildren(element?: T): ProviderResult<T[]>;
    getParent?(element: T): ProviderResult<T>;
    resolveTreeItem?(item: TreeItem, element: T, token: CancellationToken): ProviderResult<TreeItem>;
  }

  export interface TreeView<T> extends Disposable {
    readonly onDidExpandElement: Event<TreeViewExpansionEvent<T>>;
    readonly onDidCollapseElement: Event<TreeViewExpansionEvent<T>>;
    readonly selection: readonly T[];
    readonly visible: boolean;
    reveal(element: T, options?: { select?: boolean; focus?: boolean; expand?: boolean | number }): Thenable<void>;
  }

  export interface TreeViewExpansionEvent<T> {
    readonly element: T;
  }

  export type DocumentSelector = DocumentFilter | string | (DocumentFilter | string)[];

  export interface DocumentFilter {
    language?: string;
    scheme?: string;
    pattern?: string;
  }

  export type ProviderResult<T> = T | undefined | null | Thenable<T | undefined | null>;

  export interface ExtensionContext {
    readonly subscriptions: { dispose(): any }[];
    readonly extensionPath: string;
  }

  export interface SourceControlInputBox {
    value: string;
    placeholder: string;
  }

  export interface SourceControlResourceDecorations {
    strikeThrough?: boolean;
    faded?: boolean;
    tooltip?: string;
  }

  export interface SourceControlResourceState {
    readonly resourceUri: Uri;
    readonly command?: Command;
    readonly decorations?: SourceControlResourceDecorations;
  }

  export interface SourceControlResourceGroup {
    readonly id: string;
    label: string;
    hideWhenEmpty?: boolean;
    resourceStates: SourceControlResourceState[];
    dispose(): void;
  }

  export interface SourceControl {
    readonly id: string;
    readonly label: string;
    readonly rootUri?: Uri;
    readonly inputBox: SourceControlInputBox;
    statusBarCommands?: Command[];
    acceptInputCommand?: Command;
    createResourceGroup(id: string, label: string): SourceControlResourceGroup;
    dispose(): void;
  }

  export interface TextDocumentContentProvider {
    readonly onDidChange?: Event<Uri>;
    provideTextDocumentContent(uri: Uri, token?: CancellationToken): ProviderResult<string>;
  }

  export interface WebviewOptions {
    enableScripts?: boolean;
    localResourceRoots?: readonly Uri[];
  }

  export interface WebviewPanelOptions {
    enableFindWidget?: boolean;
    retainContextWhenHidden?: boolean;
  }

  export interface Webview {
    options: WebviewOptions;
    html: string;
    onDidReceiveMessage: Event<any>;
    postMessage(message: any): Thenable<boolean>;
    asWebviewUri(localResource: Uri): Uri;
  }

  export interface WebviewPanel {
    readonly viewType: string;
    title: string;
    webview: Webview;
    readonly onDidDispose: Event<void>;
    reveal(viewColumn?: ViewColumn, preserveFocus?: boolean): void;
    dispose(): any;
  }

  export interface WebviewViewResolveContext {
    readonly state?: any;
  }

  export interface WebviewView {
    readonly viewType: string;
    webview: Webview;
    title?: string;
    description?: string;
    onDidDispose: Event<void>;
  }

  export interface WebviewViewProvider {
    resolveWebviewView(
      webviewView: WebviewView,
      context: WebviewViewResolveContext,
      token: CancellationToken
    ): Thenable<void> | void;
  }

  export enum QuickPickItemKind {
    Separator = -1,
    Default = 0
  }

  export interface QuickPickItem {
    label: string;
    description?: string;
    detail?: string;
    picked?: boolean;
    alwaysShow?: boolean;
    kind?: QuickPickItemKind;
  }

  export interface QuickPickOptions {
    title?: string;
    matchOnDescription?: boolean;
    matchOnDetail?: boolean;
    placeHolder?: string;
    canPickMany?: boolean;
  }

  export enum StatusBarAlignment {
    Left = 1,
    Right = 2
  }

  export interface StatusBarItem {
    alignment: StatusBarAlignment;
    priority?: number;
    text: string;
    tooltip?: string | MarkdownString;
    color?: string;
    command?: string | Command;
    show(): void;
    hide(): void;
    dispose(): void;
  }

  export enum ViewColumn {
    Active = -1,
    Beside = -2,
    One = 1,
    Two = 2,
    Three = 3
  }

  export enum ProgressLocation {
    SourceControl = 1,
    Window = 10,
    Notification = 15
  }

  export interface ProgressOptions {
    location: ProgressLocation;
    title?: string;
    cancellable?: boolean;
  }

  export interface Progress<T> {
    report(value: T): void;
  }

  export interface FileSystemWatcher extends Disposable {
    readonly onDidChange: Event<Uri>;
    readonly onDidCreate: Event<Uri>;
    readonly onDidDelete: Event<Uri>;
  }

  export interface WorkspaceFolder {
    readonly uri: Uri;
    readonly name: string;
    readonly index: number;
  }

  export interface WorkspaceConfiguration {
    get<T>(section: string): T | undefined;
    get<T>(section: string, defaultValue: T): T;
    has(section: string): boolean;
    update(section: string, value: any, configurationTarget?: boolean): Thenable<void>;
  }

  export namespace workspace {
    export const workspaceFolders: readonly WorkspaceFolder[] | undefined;
    export function getConfiguration(section?: string, scope?: Uri | null): WorkspaceConfiguration;
    export function openTextDocument(uriOrFileName: Uri | string | { language?: string; content?: string }): Thenable<TextDocument>;
    export function registerTextDocumentContentProvider(scheme: string, provider: TextDocumentContentProvider): Disposable;
    export function findFiles(include: string, exclude?: string, maxResults?: number, token?: CancellationToken): Thenable<Uri[]>;
    export function createFileSystemWatcher(globPattern: string): FileSystemWatcher;
  }

  export namespace window {
    export function createStatusBarItem(alignment?: StatusBarAlignment, priority?: number): StatusBarItem;
    export function createWebviewPanel(
      viewType: string,
      title: string,
      showOptions: ViewColumn | { viewColumn: ViewColumn; preserveFocus?: boolean },
      options?: WebviewOptions & WebviewPanelOptions
    ): WebviewPanel;
    export function registerWebviewViewProvider(
      viewId: string,
      provider: WebviewViewProvider,
      options?: { webviewOptions?: { retainContextWhenHidden?: boolean } }
    ): Disposable;
    export function registerTreeDataProvider<T>(viewId: string, treeDataProvider: TreeDataProvider<T>): Disposable;
    export function createTreeView<T>(viewId: string, options: { treeDataProvider: TreeDataProvider<T> }): TreeView<T>;
    export function showInformationMessage<T extends string>(message: string, ...items: T[]): Thenable<T | undefined>;
    export function showWarningMessage<T extends string>(message: string, ...items: T[]): Thenable<T | undefined>;
    export function showErrorMessage<T extends string>(message: string, ...items: T[]): Thenable<T | undefined>;
    export function showInputBox(options?: {
      title?: string;
      value?: string;
      prompt?: string;
      placeHolder?: string;
      password?: boolean;
    }): Thenable<string | undefined>;
    export function showQuickPick<T extends QuickPickItem>(
      items: T[] | Thenable<T[]>,
      options?: QuickPickOptions,
      token?: CancellationToken
    ): Thenable<T | undefined>;
    export function showTextDocument(
      document: TextDocument,
      options?: { viewColumn?: ViewColumn; preserveFocus?: boolean; preview?: boolean }
    ): Thenable<TextEditor>;
    export function withProgress<R>(
      options: ProgressOptions,
      task: (progress: Progress<{ message?: string; increment?: number }>, token: CancellationToken) => Thenable<R>
    ): Thenable<R>;
  }

  export namespace scm {
    export function createSourceControl(id: string, label: string, rootUri?: Uri): SourceControl;
  }

  export namespace commands {
    export function registerCommand(command: string, callback: (...args: any[]) => any, thisArgs?: any): Disposable;
    export function executeCommand<T = unknown>(command: string, ...rest: any[]): Thenable<T>;
  }

  export namespace languages {
    export function registerCodeLensProvider(selector: DocumentSelector, provider: CodeLensProvider): Disposable;
    export function registerHoverProvider(selector: DocumentSelector, provider: HoverProvider): Disposable;
  }

  export namespace env {
    export const clipboard: {
      readText(): Thenable<string>;
      writeText(value: string): Thenable<void>;
    };
    export function openExternal(target: Uri): Thenable<boolean>;
  }
}
