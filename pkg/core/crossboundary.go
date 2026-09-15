package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// BreakageSeverity classifies the severity of a cross-boundary contract breakage.
type BreakageSeverity string

const (
	BreakageFatal   BreakageSeverity = "FATAL"
	BreakageError   BreakageSeverity = "BREAKING"
	BreakageWarning BreakageSeverity = "WARNING"
)

// BreakageType designates the category of contract breakage.
type BreakageType string

const (
	BreakageRouteRemoved        BreakageType = "ROUTE_REMOVED"
	BreakageRouteMethodMismatch BreakageType = "ROUTE_METHOD_MISMATCH"
	BreakageParamMismatch       BreakageType = "PARAM_MISMATCH"
	BreakageEnvVarMissing       BreakageType = "ENV_VAR_MISSING"
	BreakageDeployTargetMissing BreakageType = "DEPLOY_TARGET_MISSING"
	BreakageSchemaViolation     BreakageType = "SCHEMA_VIOLATION"
	BreakageRPCRemoved          BreakageType = "RPC_REMOVED"
	BreakageTableRemoved        BreakageType = "TABLE_REMOVED"
)

// ContractBreakage details a breaking change or semantic mismatch across language boundaries.
type ContractBreakage struct {
	Severity     BreakageSeverity `json:"severity"`
	Type         BreakageType     `json:"type"`
	SourceNodeID string           `json:"source_node_id"`
	TargetNodeID string           `json:"target_node_id"`
	EdgeType     EdgeType         `json:"edge_type"`
	Description  string           `json:"description"`
}

// EdgeFilterPredicate defines an optional filter to suppress cross-boundary edges.
type EdgeFilterPredicate interface {
	ShouldIgnoreEdge(edge CrossBoundaryEdge, sourceNode, targetNode *ASTSymbolNode, sourcePath, targetPath string) bool
}

// CrossBoundaryLinker resolves multi-language dependencies and builds semantic cross-boundary edges.
type CrossBoundaryLinker struct {
	filter EdgeFilterPredicate
}

// NewCrossBoundaryLinker creates a new instance of CrossBoundaryLinker.
func NewCrossBoundaryLinker() *CrossBoundaryLinker {
	return &CrossBoundaryLinker{}
}

// SetFilter configures an EdgeFilterPredicate to suppress unwanted edges.
func (l *CrossBoundaryLinker) SetFilter(f EdgeFilterPredicate) {
	l.filter = f
}

// LinkWorkspace inspects components and symbol nodes across all languages to produce valid CrossBoundaryEdge instances.
func (l *CrossBoundaryLinker) LinkWorkspace(components []*ComponentNode, symbolNodes map[string]*ASTSymbolNode) ([]CrossBoundaryEdge, error) {
	return l.LinkWorkspaceWithFilter(components, symbolNodes, l.filter)
}

// LinkWorkspaceWithFilter inspects components and symbol nodes with an explicit EdgeFilterPredicate.
func (l *CrossBoundaryLinker) LinkWorkspaceWithFilter(components []*ComponentNode, symbolNodes map[string]*ASTSymbolNode, filter EdgeFilterPredicate) ([]CrossBoundaryEdge, error) {
	var edges []CrossBoundaryEdge
	edgeSet := make(map[string]bool)

	// Map symbol node IDs and component IDs to file paths
	nodePathMap := make(map[string]string)
	for _, comp := range components {
		p := comp.Metadata["file_path"]
		if p == "" {
			p = comp.Name
		}
		nodePathMap[comp.ComponentID] = p
		for _, symID := range comp.SymbolNodes {
			nodePathMap[symID] = p
		}
	}

	addEdge := func(edge CrossBoundaryEdge) {
		srcNode := symbolNodes[edge.SourceNodeID]
		tgtNode := symbolNodes[edge.TargetNodeID]

		// 1. Check inline comment metadata on source or target node
		if checkInlineMetadata(srcNode, edge) || checkInlineMetadata(tgtNode, edge) {
			return
		}

		// 2. Check external filter if provided
		if filter != nil {
			srcPath := nodePathMap[edge.SourceNodeID]
			tgtPath := nodePathMap[edge.TargetNodeID]
			if filter.ShouldIgnoreEdge(edge, srcNode, tgtNode, srcPath, tgtPath) {
				return
			}
		}

		key := HashCrossBoundaryEdge(&edge)
		if !edgeSet[key] {
			edgeSet[key] = true
			edges = append(edges, edge)
		}
	}

	// 1. Group symbols across all languages
	var apiRoutes []*ASTSymbolNode
	var apiConsumers []*ASTSymbolNode
	var backendEnvAccessors []*ASTSymbolNode
	var sqlTables []*ASTSymbolNode
	var dbQueryAccessors []*ASTSymbolNode
	var protoRPCs []*ASTSymbolNode
	var rpcImplementors []*ASTSymbolNode
	var hclResources []*ASTSymbolNode
	var hclVariables []*ASTSymbolNode

	for _, node := range symbolNodes {
		switch node.Language {
		case LangGo, LangPython, LangRust, LangJava, LangCpp, LangC, LangKotlin, LangCSharp, LangRuby, LangPHP, LangElixir:
			// Routes
			if node.NodeType == "RouteBinding" || (node.ASTMetadata != nil && node.ASTMetadata["route_path"] != "") {
				apiRoutes = append(apiRoutes, node)
			}
			// Env accessors
			if containsEnvAccess(node) {
				backendEnvAccessors = append(backendEnvAccessors, node)
			}
			// Database queries / JPA Entities
			if isDBQueryAccessor(node) {
				dbQueryAccessors = append(dbQueryAccessors, node)
			}
			// gRPC / RPC Implementors
			if isRPCImplementor(node) {
				rpcImplementors = append(rpcImplementors, node)
			}
			// API Clients in Kotlin
			if node.NodeType == "ApiClientCall" {
				apiConsumers = append(apiConsumers, node)
			}

		case LangTypeScript, LangSwift:
			if node.NodeType == "ApiClientCall" {
				apiConsumers = append(apiConsumers, node)
			}

		case LangSQL:
			if node.NodeType == "CreateTableStatement" {
				sqlTables = append(sqlTables, node)
			}

		case LangProtobuf:
			if node.NodeType == "RPCMethod" || node.NodeType == "Service" {
				protoRPCs = append(protoRPCs, node)
			}

		case LangHCL:
			if node.NodeType == "ResourceBlock" {
				hclResources = append(hclResources, node)
			} else if node.NodeType == "VariableBlock" {
				hclVariables = append(hclVariables, node)
			}

		case LangDockerfile:
			if node.NodeType == "EnvBinding" {
				hclVariables = append(hclVariables, node)
			} else if node.NodeType == "BaseImage" || node.NodeType == "PortExpose" {
				hclResources = append(hclResources, node)
			}

		case LangGraphQL:
			if node.NodeType == "FieldDefinition" || node.NodeType == "ObjectType" || node.NodeType == "Operation" {
				apiRoutes = append(apiRoutes, node)
			}
		}
	}

	// 2. Resolve CONSUMES_API Edges (Frontend / Client calls -> Backend Routes)
	for _, consumer := range apiConsumers {
		cMethod, cPath := parseEndpoint(consumer.Identifier)
		for _, route := range apiRoutes {
			rMethod, rPath := extractRouteEndpoint(route)
			if (cMethod == "ANY" || rMethod == "ANY" || cMethod == rMethod) && pathsMatch(cPath, rPath) {
				edge := CrossBoundaryEdge{
					SourceNodeID: consumer.NodeID,
					TargetNodeID: route.NodeID,
					Type:         EdgeConsumesAPI,
					Metadata: map[string]string{
						"consumer_endpoint": cPath,
						"backend_endpoint":  rPath,
						"http_method":       rMethod,
					},
				}
				addEdge(edge)
			}
		}
	}

	// 3. Resolve QUERIES_TABLE Edges (Backend Code / JPA / SQLx -> SQL CreateTableStatement)
	for _, dbAcc := range dbQueryAccessors {
		targetTables := extractTargetTables(dbAcc)
		for _, tbl := range sqlTables {
			tblName := extractTableName(tbl)
			for _, target := range targetTables {
				if strings.EqualFold(tblName, target) || strings.HasSuffix(strings.ToLower(tblName), "."+strings.ToLower(target)) {
					edge := CrossBoundaryEdge{
						SourceNodeID: dbAcc.NodeID,
						TargetNodeID: tbl.NodeID,
						Type:         EdgeQueriesTable,
						Metadata: map[string]string{
							"table_name": tblName,
							"accessor":   dbAcc.Identifier,
						},
					}
					addEdge(edge)
				}
			}
		}
	}

	// 4. Resolve IMPLEMENTS_RPC Edges (Backend gRPC Service / Handler -> Protobuf RPCMethod / Service)
	for _, impl := range rpcImplementors {
		implName := extractRPCName(impl)
		for _, proto := range protoRPCs {
			protoName := extractProtoRPCName(proto)
			if strings.EqualFold(implName, protoName) || strings.HasSuffix(strings.ToLower(protoName), "."+strings.ToLower(implName)) || strings.Contains(strings.ToLower(impl.Identifier), strings.ToLower(protoName)) {
				edge := CrossBoundaryEdge{
					SourceNodeID: impl.NodeID,
					TargetNodeID: proto.NodeID,
					Type:         EdgeImplementsRPC,
					Metadata: map[string]string{
						"rpc_name":    protoName,
						"implementor": impl.Identifier,
					},
				}
				addEdge(edge)
			}
		}
	}

	// 5. Resolve DEPLOYS_TO Edges (Service Components -> HCL Resources)
	for _, comp := range components {
		if comp.Type == CompService || comp.Type == CompDatabase {
			for _, hclRes := range hclResources {
				if isDeploymentTargetFor(comp.Name, hclRes) {
					edge := CrossBoundaryEdge{
						SourceNodeID: comp.ComponentID,
						TargetNodeID: hclRes.NodeID,
						Type:         EdgeDeploysTo,
						Metadata: map[string]string{
							"service_name": comp.Name,
							"resource_id":  hclRes.Identifier,
						},
					}
					addEdge(edge)
				}
			}
		}
	}

	// 6. Resolve BINDS_ENV Edges (HCL Env Bindings -> Backend Env Accessors)
	for _, hclRes := range hclResources {
		envBindings := extractHCLEnvVars(hclRes)
		for _, bNode := range backendEnvAccessors {
			bEnvs := extractAccessedEnvs(bNode)
			for _, bEnv := range bEnvs {
				if val, ok := envBindings[bEnv]; ok {
					edge := CrossBoundaryEdge{
						SourceNodeID: hclRes.NodeID,
						TargetNodeID: bNode.NodeID,
						Type:         EdgeBindsEnv,
						Metadata: map[string]string{
							"env_var":   bEnv,
							"env_value": val,
						},
					}
					addEdge(edge)
				}
			}
		}
	}

	// Sort edges deterministically
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].SourceNodeID != edges[j].SourceNodeID {
			return edges[i].SourceNodeID < edges[j].SourceNodeID
		}
		if edges[i].TargetNodeID != edges[j].TargetNodeID {
			return edges[i].TargetNodeID < edges[j].TargetNodeID
		}
		return edges[i].Type < edges[j].Type
	})

	return edges, nil
}

// DetectContractBreakages compares old and new graph versions to identify breaking contract violations.
func DetectContractBreakages(
	oldEdges []CrossBoundaryEdge,
	newEdges []CrossBoundaryEdge,
	oldNodes map[string]*ASTSymbolNode,
	newNodes map[string]*ASTSymbolNode,
) []ContractBreakage {
	var breakages []ContractBreakage

	newEdgeMap := make(map[string]CrossBoundaryEdge)
	for _, e := range newEdges {
		key := HashCrossBoundaryEdge(&e)
		newEdgeMap[key] = e
	}

	for _, oldEdge := range oldEdges {
		key := HashCrossBoundaryEdge(&oldEdge)
		_, existsInNew := newEdgeMap[key]

		oldSrcNode := oldNodes[oldEdge.SourceNodeID]
		oldTgtNode := oldNodes[oldEdge.TargetNodeID]

		switch oldEdge.Type {
		case EdgeConsumesAPI:
			if !existsInNew {
				tgtStillExists := false
				if oldTgtNode != nil {
					for _, n := range newNodes {
						if n.Identifier == oldTgtNode.Identifier && n.NodeType == oldTgtNode.NodeType {
							tgtStillExists = true
							break
						}
					}
				}

				if !tgtStillExists {
					breakages = append(breakages, ContractBreakage{
						Severity:     BreakageError,
						Type:         BreakageRouteRemoved,
						SourceNodeID: oldEdge.SourceNodeID,
						TargetNodeID: oldEdge.TargetNodeID,
						EdgeType:     EdgeConsumesAPI,
						Description: fmt.Sprintf("API endpoint consumed by %s was removed or altered in backend: %s",
							nodeDesc(oldSrcNode), nodeDesc(oldTgtNode)),
					})
				}
			}

		case EdgeQueriesTable:
			if !existsInNew {
				tblName := oldEdge.Metadata["table_name"]
				breakages = append(breakages, ContractBreakage{
					Severity:     BreakageError,
					Type:         BreakageTableRemoved,
					SourceNodeID: oldEdge.SourceNodeID,
					TargetNodeID: oldEdge.TargetNodeID,
					EdgeType:     EdgeQueriesTable,
					Description: fmt.Sprintf("Database table '%s' queried by %s was modified or deleted",
						tblName, nodeDesc(oldSrcNode)),
				})
			}

		case EdgeImplementsRPC:
			if !existsInNew {
				rpcName := oldEdge.Metadata["rpc_name"]
				breakages = append(breakages, ContractBreakage{
					Severity:     BreakageError,
					Type:         BreakageRPCRemoved,
					SourceNodeID: oldEdge.SourceNodeID,
					TargetNodeID: oldEdge.TargetNodeID,
					EdgeType:     EdgeImplementsRPC,
					Description: fmt.Sprintf("Protobuf RPC definition '%s' implemented by %s was changed or removed",
						rpcName, nodeDesc(oldSrcNode)),
				})
			}

		case EdgeBindsEnv:
			if !existsInNew {
				envName := oldEdge.Metadata["env_var"]
				breakages = append(breakages, ContractBreakage{
					Severity:     BreakageError,
					Type:         BreakageEnvVarMissing,
					SourceNodeID: oldEdge.SourceNodeID,
					TargetNodeID: oldEdge.TargetNodeID,
					EdgeType:     EdgeBindsEnv,
					Description: fmt.Sprintf("Environment variable binding '%s' required by %s was removed in infra resource %s",
						envName, nodeDesc(oldTgtNode), nodeDesc(oldSrcNode)),
				})
			}

		case EdgeDeploysTo:
			if !existsInNew {
				breakages = append(breakages, ContractBreakage{
					Severity:     BreakageWarning,
					Type:         BreakageDeployTargetMissing,
					SourceNodeID: oldEdge.SourceNodeID,
					TargetNodeID: oldEdge.TargetNodeID,
					EdgeType:     EdgeDeploysTo,
					Description: fmt.Sprintf("Deployment target edge from %s to %s was detached",
						oldEdge.SourceNodeID, oldEdge.TargetNodeID),
				})
			}
		}
	}

	return breakages
}

func nodeDesc(n *ASTSymbolNode) string {
	if n == nil {
		return "unknown"
	}
	return fmt.Sprintf("%s (%s)", n.Identifier, n.NodeType)
}

var endpointRegex = regexp.MustCompile(`\b(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD|ANY|GRPC)\s+([^\s]+)`)

func parseEndpoint(ident string) (method, path string) {
	if match := endpointRegex.FindStringSubmatch(ident); match != nil {
		return strings.ToUpper(match[1]), match[2]
	}
	parts := strings.Fields(ident)
	if len(parts) >= 2 {
		return strings.ToUpper(parts[0]), parts[1]
	}
	if len(parts) == 1 {
		return "ANY", parts[0]
	}
	return "ANY", ""
}

func extractRouteEndpoint(node *ASTSymbolNode) (method, path string) {
	if node.ASTMetadata != nil {
		m := node.ASTMetadata["route_method"]
		p := node.ASTMetadata["route_path"]
		if m != "" && p != "" {
			return strings.ToUpper(m), p
		}
	}
	return parseEndpoint(node.Identifier)
}

func pathsMatch(p1, p2 string) bool {
	norm1 := normalizePath(p1)
	norm2 := normalizePath(p2)
	return norm1 == norm2
}

var paramRegex = regexp.MustCompile(`\{[a-zA-Z0-9_]+\}|:[a-zA-Z0-9_]+`)

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	return paramRegex.ReplaceAllString(p, "{param}")
}

func isDBQueryAccessor(node *ASTSymbolNode) bool {
	if node.ASTMetadata != nil {
		if node.ASTMetadata["table_name"] != "" || node.ASTMetadata["jpa_query"] != "" || node.ASTMetadata["sqlx_query"] != "" {
			return true
		}
	}
	// Check payload for sql or table names
	if len(node.ASTPayload) > 0 {
		var data map[string]interface{}
		if err := json.Unmarshal(node.ASTPayload, &data); err == nil {
			if _, ok := data["table_name"]; ok {
				return true
			}
			if _, ok := data["jpa_query"]; ok {
				return true
			}
			if _, ok := data["sql_tables"]; ok {
				return true
			}
		}
	}
	return false
}

func extractTargetTables(node *ASTSymbolNode) []string {
	var tables []string
	if node.ASTMetadata != nil {
		if tbl := node.ASTMetadata["table_name"]; tbl != "" {
			tables = append(tables, tbl)
		}
	}

	if len(node.ASTPayload) > 0 {
		var data map[string]interface{}
		if err := json.Unmarshal(node.ASTPayload, &data); err == nil {
			if t, ok := data["table_name"].(string); ok && t != "" {
				tables = append(tables, t)
			}
			if q, ok := data["jpa_query"].(string); ok && q != "" {
				fromReg := regexp.MustCompile(`(?i)(?:FROM|JOIN|INTO|UPDATE)\s+([a-zA-Z0-9_]+)`)
				matches := fromReg.FindAllStringSubmatch(q, -1)
				for _, m := range matches {
					tables = append(tables, m[1])
				}
			}
			if qArr, ok := data["sql_tables"].([]interface{}); ok {
				for _, it := range qArr {
					if s, ok := it.(string); ok && s != "" {
						tables = append(tables, s)
					}
				}
			}
		}
	}
	return tables
}

func extractTableName(tblNode *ASTSymbolNode) string {
	if tblNode.ASTMetadata != nil && tblNode.ASTMetadata["table_name"] != "" {
		return tblNode.ASTMetadata["table_name"]
	}
	ident := tblNode.Identifier
	ident = strings.TrimPrefix(ident, "table:")
	parts := strings.Split(ident, ".")
	return parts[len(parts)-1]
}

func isRPCImplementor(node *ASTSymbolNode) bool {
	if node.ASTMetadata != nil && (node.ASTMetadata["implements_rpc"] != "" || node.ASTMetadata["service"] != "") {
		return true
	}
	if strings.Contains(node.Identifier, "Server") || strings.Contains(node.Identifier, "Service") || strings.Contains(node.NodeType, "RPC") {
		return true
	}
	return false
}

func extractRPCName(node *ASTSymbolNode) string {
	if node.ASTMetadata != nil && node.ASTMetadata["implements_rpc"] != "" {
		return node.ASTMetadata["implements_rpc"]
	}
	ident := node.Identifier
	parts := strings.Split(ident, ".")
	return parts[len(parts)-1]
}

func extractProtoRPCName(proto *ASTSymbolNode) string {
	ident := strings.TrimPrefix(proto.Identifier, "rpc:")
	ident = strings.TrimPrefix(ident, "service:")
	parts := strings.Split(ident, ".")
	return parts[len(parts)-1]
}

func isDeploymentTargetFor(serviceName string, hclRes *ASTSymbolNode) bool {
	cleanServ := strings.ToLower(strings.ReplaceAll(serviceName, "-", "_"))
	cleanIdent := strings.ToLower(strings.ReplaceAll(hclRes.Identifier, "-", "_"))

	if strings.Contains(cleanIdent, cleanServ) {
		return true
	}

	if len(hclRes.ASTPayload) > 0 {
		var payloadMap map[string]interface{}
		if err := json.Unmarshal(hclRes.ASTPayload, &payloadMap); err == nil {
			if labels, ok := payloadMap["labels"].([]interface{}); ok {
				for _, l := range labels {
					if strL, ok := l.(string); ok {
						cleanL := strings.ToLower(strings.ReplaceAll(strL, "-", "_"))
						if strings.Contains(cleanL, cleanServ) {
							return true
						}
					}
				}
			}
		}
		cleanPayload := strings.ToLower(strings.ReplaceAll(string(hclRes.ASTPayload), "-", "_"))
		if strings.Contains(cleanPayload, cleanServ) {
			return true
		}
	}
	return false
}

func containsEnvAccess(n *ASTSymbolNode) bool {
	if len(n.ASTPayload) == 0 {
		return false
	}
	var data map[string]interface{}
	if err := json.Unmarshal(n.ASTPayload, &data); err == nil {
		if envs, ok := data["env_vars_accessed"].([]interface{}); ok && len(envs) > 0 {
			return true
		}
	}
	return false
}

func extractAccessedEnvs(n *ASTSymbolNode) []string {
	var envs []string
	if len(n.ASTPayload) == 0 {
		return envs
	}
	var data map[string]interface{}
	if err := json.Unmarshal(n.ASTPayload, &data); err == nil {
		if arr, ok := data["env_vars_accessed"].([]interface{}); ok {
			for _, item := range arr {
				if s, ok := item.(string); ok && s != "" {
					envs = append(envs, s)
				}
			}
		}
	}
	return envs
}

func extractHCLEnvVars(n *ASTSymbolNode) map[string]string {
	envVars := make(map[string]string)
	if len(n.ASTPayload) == 0 {
		return envVars
	}

	var block map[string]interface{}
	if err := json.Unmarshal(n.ASTPayload, &block); err == nil {
		collectEnvsRecursive(block, envVars)
	}
	return envVars
}

func collectEnvsRecursive(data map[string]interface{}, out map[string]string) {
	if blockType, ok := data["block_type"].(string); ok && blockType == "env" {
		if attrs, ok := data["attributes"].(map[string]interface{}); ok {
			var nameVal, valVal string
			if nAttr, ok := attrs["name"].(map[string]interface{}); ok {
				if v, ok := nAttr["value"].(string); ok {
					nameVal = strings.Trim(v, `"`)
				}
			}
			if vAttr, ok := attrs["value"].(map[string]interface{}); ok {
				if v, ok := vAttr["value"].(string); ok {
					valVal = strings.Trim(v, `"`)
				}
			}
			if nameVal != "" {
				out[nameVal] = valVal
			}
		}
	}

	if nested, ok := data["nested_blocks"].([]interface{}); ok {
		for _, item := range nested {
			if subMap, ok := item.(map[string]interface{}); ok {
				collectEnvsRecursive(subMap, out)
			}
		}
	}
}

func checkInlineMetadata(node *ASTSymbolNode, edge CrossBoundaryEdge) bool {
	if node == nil || node.ASTMetadata == nil {
		return false
	}
	directive := node.ASTMetadata["cosm:ignore-edge"]
	if directive == "" {
		directive = node.ASTMetadata["ignore-edge"]
	}
	if directive == "" {
		return false
	}
	parts := strings.Split(directive, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.EqualFold(p, "ALL") || strings.EqualFold(p, "*") {
			return true
		}
		if strings.EqualFold(p, string(edge.Type)) {
			return true
		}
		subParts := strings.SplitN(p, ":", 2)
		if len(subParts) == 2 {
			t := strings.TrimSpace(subParts[0])
			spec := strings.TrimSpace(subParts[1])
			if strings.EqualFold(t, string(edge.Type)) {
				if edge.Type == EdgeBindsEnv && (edge.Metadata["env_var"] == spec || spec == "*") {
					return true
				}
				if edge.Type == EdgeConsumesAPI && (edge.Metadata["consumer_endpoint"] == spec || edge.Metadata["backend_endpoint"] == spec || spec == "*") {
					return true
				}
				if edge.Type == EdgeQueriesTable && (edge.Metadata["table_name"] == spec || spec == "*") {
					return true
				}
			}
		}
	}
	return false
}

