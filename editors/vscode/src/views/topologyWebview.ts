import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import { CosmClient } from '../client/cosmClient';
import { FullTopologyGraph, TopologyNode } from '../types';

export class TopologyWebviewManager implements vscode.WebviewViewProvider, vscode.Disposable {
  public static readonly viewType = 'cosm.topologyView';
  private client: CosmClient;
  private currentPanel: vscode.WebviewPanel | undefined;
  private webviewView: vscode.WebviewView | undefined;
  private disposables: vscode.Disposable[] = [];

  constructor(client: CosmClient) {
    this.client = client;
  }

  /**
   * Implements vscode.WebviewViewProvider for the sidebar panel.
   */
  public resolveWebviewView(
    webviewView: vscode.WebviewView,
    _context: vscode.WebviewViewResolveContext,
    _token: vscode.CancellationToken
  ): void {
    this.webviewView = webviewView;
    webviewView.webview.options = {
      enableScripts: true,
      localResourceRoots: [vscode.Uri.file(this.client.getWorkspaceRoot())]
    };

    this.attachMessageHandlers(webviewView.webview);
    this.refreshWebview(webviewView.webview);

    webviewView.onDidDispose(() => {
      this.webviewView = undefined;
    }, null, this.disposables);
  }

  /**
   * Opens or reveals a full-width editor tab with the interactive 3-tier architecture canvas.
   */
  public async openEditorPanel(): Promise<void> {
    if (this.currentPanel) {
      this.currentPanel.reveal(vscode.ViewColumn.One);
      await this.refreshWebview(this.currentPanel.webview);
      return;
    }

    this.currentPanel = vscode.window.createWebviewPanel(
      'cosmTopologyPanel',
      'Cosm: Cross-Domain Architecture Canvas',
      vscode.ViewColumn.One,
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        localResourceRoots: [vscode.Uri.file(this.client.getWorkspaceRoot())]
      }
    );

    this.attachMessageHandlers(this.currentPanel.webview);
    await this.refreshWebview(this.currentPanel.webview);

    this.currentPanel.onDidDispose(() => {
      this.currentPanel = undefined;
    }, null, this.disposables);
  }

  /**
   * Re-fetches current universe topology and pushes updated state to webview.
   */
  public async refresh(): Promise<void> {
    if (this.currentPanel) {
      await this.refreshWebview(this.currentPanel.webview);
    }
    if (this.webviewView) {
      await this.refreshWebview(this.webviewView.webview);
    }
  }

  private async refreshWebview(webview: vscode.Webview): Promise<void> {
    try {
      const topology = await this.client.getTopology();
      webview.html = this.generateHtml(topology);
    } catch (err: any) {
      webview.html = `<html><body><div style="padding: 20px; color: red;">Error rendering topology: ${err.message}</div></body></html>`;
    }
  }

  private attachMessageHandlers(webview: vscode.Webview): void {
    webview.onDidReceiveMessage(
      async (message: any) => {
        switch (message.type) {
          case 'navigateToFile':
            await this.handleNavigate(message.filePath || message.target, message.symbolName, message.nodeId);
            break;

          case 'auditBlastRadius':
            await this.handleBlastRadius(message.nodeId, webview);
            break;

          case 'shipPreview':
            await vscode.commands.executeCommand('cosm.shipPreview');
            break;

          case 'refresh':
            await this.refresh();
            break;
        }
      },
      null,
      this.disposables
    );
  }

  private async handleNavigate(filePath?: string, symbolName?: string, nodeId?: string): Promise<void> {
    await vscode.commands.executeCommand('cosm.navigateToSymbol', filePath || symbolName, symbolName || nodeId);
  }

  private async handleBlastRadius(nodeId: string, webview: vscode.Webview): Promise<void> {
    try {
      const report = await this.client.getBlastRadius(nodeId);
      webview.postMessage({
        type: 'blastRadiusResult',
        nodeId,
        report
      });
    } catch (err: any) {
      vscode.window.showErrorMessage(`Blast radius audit failed: ${err.message}`);
    }
  }

  private generateHtml(graph: FullTopologyGraph): string {
    const frontendNodes = graph.frontend_nodes || [];
    const backendNodes = graph.backend_nodes || [];
    const infraNodes = graph.infra_nodes || [];

    const graphJson = JSON.stringify(graph);

    return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Cosm Architecture Topology</title>
  <style>
    :root {
      --bg: var(--vscode-editor-background, #1e1e2e);
      --fg: var(--vscode-editor-foreground, #cdd6f4);
      --card-bg: var(--vscode-sideBar-background, #181825);
      --card-border: var(--vscode-widget-border, #313244);
      --accent-fe: #89b4fa;
      --accent-be: #a6e3a1;
      --accent-infra: #cba6f7;
      --danger: #f38ba8;
      --warning: #fab387;
    }

    body {
      background-color: var(--bg);
      color: var(--fg);
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      margin: 0;
      padding: 16px;
      overflow-x: hidden;
    }

    .header-bar {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 16px;
      padding-bottom: 12px;
      border-bottom: 1px solid var(--card-border);
    }

    .title-group h2 {
      margin: 0;
      font-size: 1.2rem;
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .metrics-pills {
      display: flex;
      gap: 8px;
      font-size: 0.8rem;
    }

    .pill {
      background: var(--card-bg);
      border: 1px solid var(--card-border);
      padding: 3px 8px;
      border-radius: 12px;
    }

    .controls {
      display: flex;
      gap: 8px;
    }

    input[type="text"] {
      background: var(--card-bg);
      border: 1px solid var(--card-border);
      color: var(--fg);
      padding: 5px 10px;
      border-radius: 4px;
      outline: none;
      font-size: 0.85rem;
    }

    button {
      background: var(--vscode-button-background, #3b82f6);
      color: var(--vscode-button-foreground, #ffffff);
      border: none;
      padding: 6px 12px;
      border-radius: 4px;
      cursor: pointer;
      font-size: 0.85rem;
      display: flex;
      align-items: center;
      gap: 4px;
      transition: opacity 0.2s;
    }

    button:hover {
      opacity: 0.85;
    }

    .swimlanes-container {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
      gap: 16px;
      align-items: start;
    }

    .swimlane {
      background: var(--card-bg);
      border: 1px solid var(--card-border);
      border-radius: 8px;
      padding: 12px;
      display: flex;
      flex-direction: column;
      gap: 10px;
      min-height: 200px;
    }

    .swimlane-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding-bottom: 8px;
      border-bottom: 2px solid transparent;
      font-weight: 600;
      font-size: 0.95rem;
    }

    .swimlane.frontend .swimlane-header {
      border-bottom-color: var(--accent-fe);
      color: var(--accent-fe);
    }

    .swimlane.backend .swimlane-header {
      border-bottom-color: var(--accent-be);
      color: var(--accent-be);
    }

    .swimlane.infra .swimlane-header {
      border-bottom-color: var(--accent-infra);
      color: var(--accent-infra);
    }

    .node-list {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }

    .node-card {
      background: rgba(255, 255, 255, 0.03);
      border: 1px solid var(--card-border);
      border-radius: 6px;
      padding: 10px;
      cursor: pointer;
      transition: all 0.2s ease;
      position: relative;
    }

    .node-card:hover {
      border-color: var(--accent-fe);
      transform: translateY(-2px);
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    }

    .node-card.highlight-target {
      border-color: var(--danger) !important;
      background: rgba(243, 139, 168, 0.15);
    }

    .node-card.highlight-downstream {
      border-color: var(--warning) !important;
      background: rgba(250, 179, 135, 0.15);
    }

    .node-card.dimmed {
      opacity: 0.25;
    }

    .node-top {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      margin-bottom: 4px;
    }

    .node-name {
      font-weight: 600;
      font-size: 0.85rem;
      word-break: break-all;
    }

    .node-type-badge {
      font-size: 0.7rem;
      padding: 2px 6px;
      border-radius: 4px;
      background: rgba(255, 255, 255, 0.08);
      color: var(--fg);
    }

    .edge-badge {
      display: inline-block;
      margin-top: 6px;
      font-size: 0.7rem;
      padding: 2px 6px;
      border-radius: 4px;
      background: rgba(137, 180, 250, 0.15);
      border: 1px solid var(--accent-fe);
      color: var(--accent-fe);
    }

    .blast-modal {
      display: none;
      background: var(--card-bg);
      border: 1px solid var(--danger);
      border-radius: 8px;
      padding: 12px;
      margin-bottom: 16px;
    }

    .empty-state {
      font-size: 0.85rem;
      color: rgba(255, 255, 255, 0.4);
      text-align: center;
      padding: 20px;
    }
  </style>
</head>
<body>
  <div class="header-bar">
    <div class="title-group">
      <h2>🌌 Cosm Polyglot Architecture Topology</h2>
      <div class="metrics-pills">
        <span class="pill">Nodes: <strong>${graph.total_nodes}</strong></span>
        <span class="pill">Cross-Domain Contracts: <strong>${graph.total_edges}</strong></span>
        <span class="pill">Active Universe: <strong>universe-main</strong></span>
      </div>
    </div>
    <div class="controls">
      <input type="text" id="searchFilter" placeholder="Filter symbols..." oninput="filterNodes()">
      <button onclick="triggerShip()">🚀 Ship Preview</button>
      <button onclick="triggerRefresh()">🔄 Refresh</button>
    </div>
  </div>

  <div id="blastAlert" class="blast-modal">
    <strong id="blastTitle">⚠️ Blast Radius Impact</strong>
    <p id="blastContent" style="margin: 4px 0 0 0; font-size: 0.85rem;"></p>
  </div>

  <div class="swimlanes-container">
    <!-- Frontend Swimlane -->
    <div class="swimlane frontend">
      <div class="swimlane-header">
        <span>[1] FRONTEND TIER (React / TS)</span>
        <span>${frontendNodes.length}</span>
      </div>
      <div class="node-list" id="feList">
        ${
          frontendNodes.length === 0
            ? '<div class="empty-state">No frontend symbols detected</div>'
            : frontendNodes.map(n => renderNodeCard(n)).join('')
        }
      </div>
    </div>

    <!-- Backend Swimlane -->
    <div class="swimlane backend">
      <div class="swimlane-header">
        <span>[2] BACKEND / API TIER (Go / Python)</span>
        <span>${backendNodes.length}</span>
      </div>
      <div class="node-list" id="beList">
        ${
          backendNodes.length === 0
            ? '<div class="empty-state">No backend symbols detected</div>'
            : backendNodes.map(n => renderNodeCard(n)).join('')
        }
      </div>
    </div>

    <!-- Cloud Infra Swimlane -->
    <div class="swimlane infra">
      <div class="swimlane-header">
        <span>[3] CLOUD INFRASTRUCTURE (Terraform HCL)</span>
        <span>${infraNodes.length}</span>
      </div>
      <div class="node-list" id="infraList">
        ${
          infraNodes.length === 0
            ? '<div class="empty-state">No infrastructure resources detected</div>'
            : infraNodes.map(n => renderNodeCard(n)).join('')
        }
      </div>
    </div>
  </div>

  <script>
    const vscode = acquireVsCodeApi();
    const topologyGraph = ${graphJson};

    function renderNodeCard(node) {
      // client-side helper if needed
    }

    function navigate(filePath, symbolName, nodeId) {
      vscode.postMessage({ type: 'navigateToFile', filePath, symbolName, nodeId, target: filePath || symbolName });
    }

    function auditBlast(event, nodeId) {
      event.stopPropagation();
      vscode.postMessage({ type: 'auditBlastRadius', nodeId });
    }

    function triggerShip() {
      vscode.postMessage({ type: 'shipPreview' });
    }

    function triggerRefresh() {
      vscode.postMessage({ type: 'refresh' });
    }

    function filterNodes() {
      const q = document.getElementById('searchFilter').value.toLowerCase();
      document.querySelectorAll('.node-card').forEach(card => {
        const text = card.textContent.toLowerCase();
        card.style.display = text.includes(q) ? 'block' : 'none';
      });
    }

    function highlightCascade(nodeId) {
      // Find direct and downstream edges
      const allCards = document.querySelectorAll('.node-card');
      if (!nodeId) {
        allCards.forEach(c => c.classList.remove('highlight-target', 'highlight-downstream', 'dimmed'));
        return;
      }

      allCards.forEach(c => {
        if (c.dataset.id === nodeId) {
          c.classList.add('highlight-target');
          c.classList.remove('dimmed');
        } else {
          c.classList.add('dimmed');
        }
      });
    }

    window.addEventListener('message', event => {
      const msg = event.data;
      if (msg.type === 'blastRadiusResult') {
        const alertBox = document.getElementById('blastAlert');
        const content = document.getElementById('blastContent');
        alertBox.style.display = 'block';

        let warningHtml = '';
        if (msg.report.warnings && msg.report.warnings.length > 0) {
          warningHtml = '<div style="margin-top: 4px; color: #f38ba8; font-size: 0.75rem;">⚠️ ' + msg.report.warnings.join('<br>⚠️ ') + '</div>';
        }

        const direct = msg.report.total_direct_nodes ?? 0;
        const downstream = msg.report.total_downstream_nodes ?? 0;
        const riskPct = ((msg.report.risk_score || 0) * 100).toFixed(0);

        content.innerHTML = '<strong>' + escapeHtml(msg.report.summary || 'Impact cascade analysis') + '</strong><br>' +
          'Direct Contracts: ' + direct + ' | ' +
          'Downstream Components: ' + downstream + ' | ' +
          'Risk Score: ' + riskPct + '%' + warningHtml;
      }
    });
  </script>
</body>
</html>`;
  }

  public dispose(): void {
    if (this.currentPanel) {
      this.currentPanel.dispose();
    }
    for (const d of this.disposables) {
      d.dispose();
    }
  }
}

function renderNodeCard(n: TopologyNode): string {
  const outgoingEdges = n.outgoing || [];
  const edgeBadges = outgoingEdges.map(e => `<span class="edge-badge">🔗 ${e.edge_type}: ${e.label}</span>`).join(' ');
  const safeFilePath = escapeHtml(n.file_path || '');
  const safeName = escapeHtml(n.name || '');

  return `
    <div class="node-card" data-id="${n.id}" onclick="navigate('${safeFilePath}', '${safeName}', '${n.id}')" onmouseenter="highlightCascade('${n.id}')" onmouseleave="highlightCascade(null)">
      <div class="node-top">
        <span class="node-name">${safeName}</span>
        <span class="node-type-badge">${n.type} (${n.language})</span>
      </div>
      ${n.file_path ? `<div style="font-size: 0.72rem; opacity: 0.8; margin: 2px 0 4px 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">📄 ${safeFilePath}</div>` : ''}
      ${edgeBadges ? `<div>${edgeBadges}</div>` : ''}
      <div style="margin-top: 6px; display: flex; justify-content: flex-end;">
        <span style="font-size: 0.7rem; color: #89b4fa; cursor: pointer;" onclick="auditBlast(event, '${n.id}')">Audit Blast ➔</span>
      </div>
    </div>
  `;
}

function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}
