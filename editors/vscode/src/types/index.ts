/**
 * Core type definitions for Cosm AST SCM & Polyglot Architecture.
 * Matches domain primitives in github.com/cosmscm/cosm/pkg/core, storage, target, and lineage.
 */

export interface TokenTelemetry {
  prompt_tokens: number;
  completion_tokens: number;
  reasoning_tokens?: number;
  total_tokens?: number;
  cost_usd?: number;
  latency_ms?: number;
}

export interface TraceCarrier {
  trace_id: string;
  span_id: string;
}

export interface LineageEnvelope {
  user_id: string;
  user_prompt: string;
  session_id: string;
  orchestrator_agent_id: string;
  executing_agent_id: string;
  llm_version: string;
  generation_params: string;
  intent: string;
  timestamp: string;
  tokens?: TokenTelemetry;
  trace?: TraceCarrier;
  signature_ed25519?: string;
}

export interface ASTSymbol {
  node_id: string;
  language: string;
  node_type: string;
  identifier: string;
  signature?: string;
  docstring?: string;
  visibility?: string;
  ast_payload?: string;
  ast_metadata?: Record<string, string>;
  local_dependencies?: string[];
  dependencies?: string[];
  lineage?: LineageEnvelope;
}

export interface ComponentNode {
  component_id: string;
  name: string;
  type: string;
  language: string;
  symbol_nodes: string[];
  merkle_hash?: string;
  metadata?: Record<string, string>;
}

export interface ResolvedSymbol {
  symbol_node: ASTSymbol;
  component: ComponentNode;
  comp_index: number;
  symbol_index: number;
}

export interface CrossBoundaryEdge {
  source_node_id: string;
  target_node_id: string;
  type: 'CALLS' | 'CONSUMES_API' | 'DEPLOYS_TO' | 'BINDS_ENV' | 'DEPENDS_ON' | string;
  metadata?: Record<string, string>;
}

export interface TopologyEdge {
  target_id: string;
  edge_type: string;
  label: string;
}

export interface TopologyNode {
  id: string;
  name: string;
  tier: 'Frontend' | 'API/Backend' | 'Backend' | 'Cloud Infra' | string;
  language: string;
  type: string;
  file_path?: string;
  component_name?: string;
  outgoing?: TopologyEdge[] | null;
}

export interface FullTopologyGraph {
  frontend_nodes?: TopologyNode[];
  backend_nodes?: TopologyNode[];
  infra_nodes?: TopologyNode[];
  total_nodes: number;
  total_edges: number;
}

export interface UniverseRecord {
  universe_id: string;
  head_manifest_hash: string;
  parent_universe_id?: string;
  status: 'active' | 'archived' | 'merged' | string;
  created_at?: string;
  updated_at?: string;
}

export interface BlastRadiusReport {
  summary: string;
  total_direct_nodes: number;
  total_downstream_nodes: number;
  risk_score: number;
  target_symbol_id?: string;
  incoming_edges?: string[];
  outgoing_edges?: string[];
  affected_components?: string[];
  warnings?: string[];
  TotalDirectNodes?: number;
  TotalDownstreamNodes?: number;
  RiskScore?: number;
  Summary?: string;
}

export interface ShipResult {
  status: string;
  target: string;
  universe_id: string;
  merkle_root: string;
  artifact_id: string;
  artifact_path: string;
  size_bytes: number;
  preview_url?: string;
  healthy?: boolean;
}

export interface WorkingTreeLens {
  status?: string;
  projected_files_count?: number;
  drifted_files?: string[];
  [key: string]: any;
}

export interface CosmStatus {
  status: string;
  universe_id: string;
  merkle_root: string;
  components_count: number;
  cross_edges_count: number;
  components: string[];
  clean?: boolean;
  staged_files?: string[];
  modified_files?: string[];
  working_tree_lens?: WorkingTreeLens;
}

export interface ProposalRecord {
  proposal_id: string;
  source_universe: string;
  target_universe: string;
  title: string;
  status: string;
}

export interface StackRecord {
  change_id: string;
  universe_id: string;
  parent_change_id?: string;
  title?: string;
  status?: string;
  manifest_hash?: string;
  order_index?: number;
  author_did?: string;
}

export interface ASTTreeEdge {
  target_id: string;
  edge_type: string;
  label: string;
}

export interface ASTTreeLineage {
  user_prompt?: string;
  executing_agent_id?: string;
  intent?: string;
  timestamp?: string;
}

export interface ASTTreeSymbol {
  node_id: string;
  identifier: string;
  node_type: string;
  language: string;
  signature?: string;
  docstring?: string;
  visibility?: string;
  dependencies?: string[];
  outgoing_edges?: ASTTreeEdge[];
  lineage?: ASTTreeLineage;
}

export interface ASTTreeComponent {
  component_id: string;
  name: string;
  language: string;
  type: string;
  symbols: ASTTreeSymbol[];
}

export interface ASTTreeGraph {
  universe_id: string;
  merkle_root: string;
  total_components: number;
  total_symbols: number;
  total_edges: number;
  components: ASTTreeComponent[];
}

