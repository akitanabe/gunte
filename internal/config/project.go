package config

import (
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

func (v *validator) project(root map[string]any, order []toml.Key) ProjectConfig {
	allowed := []string{"spec_version", "project", "sources", "terms", "targets"}
	if version, ok := root["spec_version"].(int64); ok && version == 2 {
		allowed = append(allowed, "contracts")
	}
	v.unknownKeys("", root, allowed...)
	config := ProjectConfig{}
	if raw, ok := root["spec_version"]; !ok {
		v.add("spec_version", "spec_version is required")
	} else if version, ok := raw.(int64); !ok {
		v.add("spec_version", "spec_version must be an integer")
	} else {
		config.SpecVersion = int(version)
		if version != 1 && version != 2 {
			v.add("spec_version", "spec_version must be 1 or 2")
		}
	}
	config.Project = v.projectData(root["project"], config.SpecVersion)
	if config.SpecVersion == 2 {
		config.ContractFiles = v.contractFiles(root["contracts"])
	} else {
		config.ContractFiles = []string{"contracts.toml"}
	}
	config.Sources = v.sources(root["sources"], config.SpecVersion)
	targets, targetSet := v.targets(root["targets"], order, config.SpecVersion)
	config.Targets = targets
	config.Terms = v.terms(root["terms"], order, config.TargetIDs(), targetSet)
	if config.SpecVersion == 2 {
		v.validateManagedScopes(config)
	}
	return config
}

func (v *validator) contractFiles(raw any) []string {
	if raw == nil {
		return []string{"contracts.toml"}
	}
	values, ok := table(raw)
	if !ok {
		v.add("contracts", "contracts must be a table")
		return nil
	}
	v.unknownKeys("contracts", values, "files")
	rawFiles, exists := values["files"]
	if !exists {
		return []string{"contracts.toml"}
	}
	items, ok := array(rawFiles)
	if !ok {
		v.add("contracts.files", "contracts.files must be an array of strings")
		return nil
	}
	if len(items) == 0 {
		v.add("contracts.files", "contracts.files must contain at least one path")
	}
	result := make([]string, 0, len(items))
	seen := map[string]bool{}
	for index, item := range items {
		key := formatIndex("contracts.files", index)
		path, ok := stringValue(item)
		if !ok {
			v.add(key, key+" must be a string")
			continue
		}
		result = append(result, path)
		validatePath(v, key, path)
		if seen[path] {
			v.add(key, "duplicate contract path "+path)
		}
		seen[path] = true
	}
	return result
}

func (v *validator) projectData(raw any, specVersion int) Project {
	values, ok := table(raw)
	if !ok {
		v.add("project", "project table is required")
		return Project{}
	}
	allowed := []string{"id", "version"}
	if specVersion == 2 {
		allowed = append(allowed, "version_from")
	}
	v.unknownKeys("project", values, allowed...)
	project := Project{ID: requiredString(v, values, "project.id", "id")}
	if specVersion == 2 {
		version, hasVersion := values["version"]
		versionFrom, hasVersionFrom := values["version_from"]
		if hasVersion == hasVersionFrom {
			v.add("project", "exactly one of project.version or project.version_from is required")
		}
		if hasVersion {
			project.Version, _ = stringValue(version)
			if _, ok := stringValue(version); !ok {
				v.add("project.version", "project.version must be a string")
			}
		}
		if hasVersionFrom {
			project.VersionFrom, _ = stringValue(versionFrom)
			if _, ok := stringValue(versionFrom); !ok {
				v.add("project.version_from", "project.version_from must be a string")
			} else {
				validatePath(v, "project.version_from", project.VersionFrom)
			}
		}
	} else {
		project.Version = requiredString(v, values, "project.version", "version")
	}
	validateNonemptySingleLine(v, "project.id", project.ID)
	if project.Version != "" || specVersion != 2 || values["version"] != nil {
		validateNonemptySingleLine(v, "project.version", project.Version)
	}
	return project
}

func (v *validator) sources(raw any, specVersion int) Sources {
	values, ok := table(raw)
	if !ok {
		v.add("sources", "sources table is required")
		return Sources{}
	}
	allowed := []string{"files"}
	if specVersion == 2 {
		allowed = append(allowed, "managed_roots", "allow_files", "allow_dirs")
	}
	v.unknownKeys("sources", values, allowed...)
	items, ok := array(values["files"])
	if !ok {
		if _, exists := values["files"]; !exists {
			v.add("sources.files", "sources.files is required")
		} else {
			v.add("sources.files", "sources.files must be an array of strings")
		}
		return Sources{}
	}
	if len(items) == 0 {
		v.add("sources.files", "sources.files must contain at least one path")
	}
	seen := map[string]bool{}
	result := Sources{Files: make([]string, 0, len(items))}
	for i, item := range items {
		path, ok := stringValue(item)
		key := formatIndex("sources.files", i)
		if !ok {
			v.add(key, key+" must be a string")
			continue
		}
		result.Files = append(result.Files, path)
		validatePath(v, key, path)
		if seen[path] {
			v.add(key, "duplicate source path "+path)
		}
		seen[path] = true
	}
	if specVersion == 2 {
		result.ManagedRoots = v.pathList(values, "sources.managed_roots")
		result.AllowFiles = v.pathList(values, "sources.allow_files")
		result.AllowDirs = v.pathList(values, "sources.allow_dirs")
	}
	return result
}

func (v *validator) targets(raw any, order []toml.Key, specVersion int) ([]Target, map[string]bool) {
	values, ok := table(raw)
	if !ok || len(values) == 0 {
		v.add("targets", "targets must contain at least one target")
		return nil, map[string]bool{}
	}
	ids := orderedChildNames(order, "targets")
	set := map[string]bool{}
	result := make([]Target, 0, len(ids))
	for _, id := range ids {
		set[id] = true
		if !targetIDPattern.MatchString(id) {
			v.add("targets."+id, "invalid target ID "+id)
		}
		entry, ok := table(values[id])
		if !ok {
			v.add("targets."+id, "target "+id+" must be a table")
			continue
		}
		prefix := "targets." + id
		allowed := []string{"output_root", "rules"}
		if specVersion == 2 {
			allowed = append(allowed, "managed_roots", "allow_files", "allow_dirs")
		}
		v.unknownKeys(prefix, entry, allowed...)
		target := Target{ID: id, OutputRoot: requiredString(v, entry, prefix+".output_root", "output_root"), Rules: []Rule{}}
		validatePath(v, prefix+".output_root", target.OutputRoot)
		if specVersion == 2 {
			target.ManagedRoots = v.pathList(entry, prefix+".managed_roots")
			target.AllowFiles = v.pathList(entry, prefix+".allow_files")
			target.AllowDirs = v.pathList(entry, prefix+".allow_dirs")
		}
		if rawRules, exists := entry["rules"]; exists {
			rules, ok := array(rawRules)
			if !ok {
				v.add(prefix+".rules", prefix+".rules must be an array")
			} else {
				for i, rawRule := range rules {
					target.Rules = append(target.Rules, v.rule(rawRule, formatIndex(prefix+".rules", i)))
				}
			}
		}
		result = append(result, target)
	}
	return result, set
}

func (v *validator) pathList(values map[string]any, keyPath string) []string {
	leaf := keyPath[strings.LastIndexByte(keyPath, '.')+1:]
	raw, exists := values[leaf]
	if !exists {
		return nil
	}
	items, ok := array(raw)
	if !ok {
		v.add(keyPath, keyPath+" must be an array of strings")
		return nil
	}
	result := make([]string, 0, len(items))
	for index, item := range items {
		key := formatIndex(keyPath, index)
		path, ok := stringValue(item)
		if !ok {
			v.add(key, key+" must be a string")
			continue
		}
		result = append(result, path)
		validatePath(v, key, path)
	}
	return result
}

type managedScope struct {
	rootPrefix  string
	managedRoot string
}

func (v *validator) validateManagedScopes(project ProjectConfig) {
	scopes := make([]managedScope, 0)
	for index, root := range project.Sources.ManagedRoots {
		scopes = append(scopes, managedScope{
			rootPrefix:  formatIndex("sources.managed_roots", index),
			managedRoot: root,
		})
	}
	for _, target := range project.Targets {
		for index, root := range target.ManagedRoots {
			scopes = append(scopes, managedScope{
				rootPrefix:  formatIndex("targets."+target.ID+".managed_roots", index),
				managedRoot: joinRelativePath(target.OutputRoot, root),
			})
		}
	}
	for first := 0; first < len(scopes); first++ {
		for second := first + 1; second < len(scopes); second++ {
			if pathOverlaps(scopes[first].managedRoot, scopes[second].managedRoot) {
				v.add(scopes[second].rootPrefix, "managed root overlaps "+scopes[first].managedRoot)
			}
		}
	}
	for _, target := range project.Targets {
		v.validateScopeAllows("targets."+target.ID, target.OutputRoot, target.ManagedRoots, target.AllowFiles, target.AllowDirs)
	}
	v.validateScopeAllows("sources", "", project.Sources.ManagedRoots, project.Sources.AllowFiles, project.Sources.AllowDirs)
}

func (v *validator) validateScopeAllows(prefix, outputRoot string, roots, allowFiles, allowDirs []string) {
	fullRoots := roots
	fullFiles := allowFiles
	fullDirs := allowDirs
	if outputRoot != "" {
		fullRoots = prefixPaths(outputRoot, roots)
		fullFiles = prefixPaths(outputRoot, allowFiles)
		fullDirs = prefixPaths(outputRoot, allowDirs)
	}
	for index := range allowFiles {
		full := fullFiles[index]
		matches := 0
		for _, root := range fullRoots {
			if pathDescendant(root, full) {
				matches++
			}
		}
		if matches != 1 {
			v.add(formatIndex(prefix+".allow_files", index), "allow file must be under exactly one managed root")
		}
	}
	for index := range allowDirs {
		full := fullDirs[index]
		matches := 0
		for _, root := range fullRoots {
			if pathDescendant(root, full) {
				matches++
			}
		}
		if matches != 1 {
			v.add(formatIndex(prefix+".allow_dirs", index), "allow directory must be under exactly one managed root")
		}
	}
	type allowance struct {
		path string
		key  string
		dir  bool
	}
	allowances := make([]allowance, 0, len(allowFiles)+len(allowDirs))
	for index, path := range fullFiles {
		allowances = append(allowances, allowance{path: path, key: formatIndex(prefix+".allow_files", index)})
	}
	for index, path := range fullDirs {
		allowances = append(allowances, allowance{path: path, key: formatIndex(prefix+".allow_dirs", index), dir: true})
	}
	for first := 0; first < len(allowances); first++ {
		for second := first + 1; second < len(allowances); second++ {
			if allowances[first].path == allowances[second].path {
				v.add(allowances[second].key, "allow entries overlap "+allowances[first].path)
				continue
			}
			if allowances[first].dir && pathContains(allowances[first].path, allowances[second].path) {
				v.add(allowances[second].key, "allow entry is redundantly contained by allow directory "+allowances[first].path)
			}
			if allowances[second].dir && pathContains(allowances[second].path, allowances[first].path) {
				v.add(allowances[first].key, "allow entry is redundantly contained by allow directory "+allowances[second].path)
			}
		}
	}
}

func prefixPaths(prefix string, paths []string) []string {
	result := make([]string, len(paths))
	for index, value := range paths {
		result[index] = joinRelativePath(prefix, value)
	}
	return result
}

func joinRelativePath(prefix, value string) string {
	if prefix == "" {
		return value
	}
	if value == "" {
		return prefix
	}
	return prefix + "/" + value
}

func pathContains(root, value string) bool {
	return root == value || strings.HasPrefix(value, root+"/")
}

func pathDescendant(root, value string) bool {
	return root != value && pathContains(root, value)
}

func pathOverlaps(first, second string) bool {
	return pathContains(first, second) || pathContains(second, first)
}

func (v *validator) terms(raw any, order []toml.Key, targetIDs []string, targets map[string]bool) []Term {
	values, ok := table(raw)
	if raw == nil {
		return nil
	}
	if !ok {
		v.add("terms", "terms must be a table")
		return nil
	}
	names := orderedChildNames(order, "terms")
	result := make([]Term, 0, len(names))
	for _, name := range names {
		prefix := "terms." + name
		if !termNamePattern.MatchString(name) {
			v.add(prefix, "invalid term name "+name)
		}
		termValues, ok := table(values[name])
		if !ok {
			v.add(prefix, "term "+name+" must be a table")
			continue
		}
		term := Term{Name: name}
		for _, target := range targetIDs {
			value, exists := termValues[target]
			if !exists {
				v.add(prefix, "term "+name+" is missing target "+target)
				continue
			}
			text, ok := stringValue(value)
			if !ok {
				v.add(prefix+"."+target, prefix+"."+target+" must be a string")
				continue
			}
			if text == "" {
				v.add(prefix+"."+target, "term "+name+"."+target+" must be non-empty")
			}
			if strings.ContainsAny(text, "\r\n") {
				v.add(prefix+"."+target, "term "+name+"."+target+" must be a single-line string")
			}
			term.Values = append(term.Values, TargetValue{TargetID: target, Value: text})
		}
		for _, target := range sortedKeys(termValues) {
			if !targets[target] {
				v.add(prefix+"."+target, "term "+name+" has unknown target "+target)
			}
		}
		result = append(result, term)
	}
	return result
}

func validatePath(v *validator, key, path string) {
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, `\`) || !utf8.ValidString(path) {
		v.add(key, key+" must be a relative slash-separated path")
	}
	if strings.ContainsRune(path, 0) {
		v.add(key, key+" must not contain NUL")
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" {
			v.add(key, key+" must not contain empty path segments")
		}
		if segment == "." || segment == ".." {
			v.add(key, key+" must not contain . or .. path segments")
		}
	}
}

func (v *validator) rule(raw any, prefix string) Rule {
	values, ok := table(raw)
	if !ok {
		v.add(prefix, prefix+" must be a table")
		return Rule{}
	}
	v.unknownKeys(prefix, values, "match", "path", "profile", "header", "metadata", "body_field", "value_from")
	rule := Rule{
		Match:    requiredString(v, values, prefix+".match", "match"),
		Path:     requiredString(v, values, prefix+".path", "path"),
		Profile:  Profile(requiredString(v, values, prefix+".profile", "profile")),
		Metadata: []MetadataEntry{},
	}
	presence := rulePresence{
		header:    values["header"] != nil,
		metadata:  values["metadata"] != nil,
		bodyField: values["body_field"] != nil,
		valueFrom: values["value_from"] != nil,
	}
	validatePath(v, prefix+".match", rule.Match)
	validatePath(v, prefix+".path", rule.Path)
	if presence.header {
		rule.Header, _ = stringValue(values["header"])
		if _, ok := stringValue(values["header"]); !ok {
			v.add(prefix+".header", "header must be a string")
		} else {
			validateNonemptySingleLine(v, prefix+".header", rule.Header)
		}
	}
	if presence.bodyField {
		if bodyField, ok := stringValue(values["body_field"]); ok {
			rule.BodyField = bodyField
			v.validateField(prefix+".body_field", "body_field", rule.BodyField)
		} else {
			v.add(prefix+".body_field", "body_field must be a string")
		}
	}
	if presence.valueFrom {
		rule.ValueFrom, _ = stringValue(values["value_from"])
		if _, ok := stringValue(values["value_from"]); !ok {
			v.add(prefix+".value_from", "value_from must be a string")
		}
	}
	if presence.metadata {
		rule.Metadata = v.metadata(values["metadata"], prefix+".metadata")
	}
	v.validateProfile(rule, presence, prefix)
	if rule.Profile == ProfileYAML {
		for i, entry := range rule.Metadata {
			if yamlReserved[strings.ToLower(entry.Field)] {
				v.add(formatIndex(prefix+".metadata", i)+".field", "reserved YAML metadata field "+entry.Field)
			}
		}
	}
	return rule
}

func (v *validator) metadata(raw any, prefix string) []MetadataEntry {
	items, ok := array(raw)
	if !ok {
		v.add(prefix, "metadata must be an array")
		return nil
	}
	seen := map[string]bool{}
	result := make([]MetadataEntry, 0, len(items))
	for i, item := range items {
		itemPrefix := formatIndex(prefix, i)
		values, ok := table(item)
		if !ok {
			v.add(itemPrefix, itemPrefix+" must be a table")
			continue
		}
		v.unknownKeys(itemPrefix, values, "field", "from", "type", "required")
		entry := MetadataEntry{
			Field:    requiredString(v, values, itemPrefix+".field", "field"),
			From:     requiredString(v, values, itemPrefix+".from", "from"),
			Type:     MetadataType(requiredString(v, values, itemPrefix+".type", "type")),
			Required: true,
		}
		v.validateField(itemPrefix+".field", "metadata field", entry.Field)
		if seen[entry.Field] {
			v.add(itemPrefix+".field", "duplicate metadata field "+entry.Field)
		}
		seen[entry.Field] = true
		if rawRequired, exists := values["required"]; exists {
			if required, ok := boolValue(rawRequired); ok {
				entry.Required = required
			} else {
				v.add(itemPrefix+".required", "required must be a boolean")
			}
		}
		switch entry.Type {
		case MetadataString, MetadataStringList, MetadataCommaList, MetadataPlainToken:
		default:
			v.add(itemPrefix+".type", "metadata type must be one of string, string_list, comma_list, plain_token")
		}
		result = append(result, entry)
	}
	return result
}

func (v *validator) validateField(key, label, field string) {
	if !fieldPattern.MatchString(field) {
		v.add(key, "invalid "+label+" "+field)
	}
}

type rulePresence struct {
	header, metadata, bodyField, valueFrom bool
}

func (v *validator) validateProfile(rule Rule, presence rulePresence, prefix string) {
	allowed := map[Profile]map[string]bool{
		ProfileMarkdown:  {"header": true},
		ProfileYAML:      {"header": true, "metadata": true},
		ProfileTOML:      {"header": true, "metadata": true, "body_field": true},
		ProfileJSON:      {"metadata": true},
		ProfilePlainText: {"value_from": true},
	}
	fields, known := allowed[rule.Profile]
	if !known {
		v.add(prefix+".profile", "profile must be one of markdown-v1, markdown+yaml-frontmatter-v1, toml-v1, json-v1, plain-text-v1")
		return
	}
	declared := []struct {
		field   string
		present bool
	}{{"header", presence.header}, {"metadata", presence.metadata}, {"body_field", presence.bodyField}, {"value_from", presence.valueFrom}}
	for _, declaration := range declared {
		field, present := declaration.field, declaration.present
		if present && !fields[field] {
			v.add(prefix+"."+field, field+" is not allowed for "+string(rule.Profile))
		}
	}
	if rule.Profile == ProfilePlainText && !presence.valueFrom {
		v.add(prefix+".value_from", "value_from is required for plain-text-v1")
	}
	if rule.Profile == ProfileTOML && !presence.header && !presence.bodyField && len(rule.Metadata) == 0 {
		v.add(prefix, "toml-v1 rule needs a content producer")
	}
	if rule.BodyField != "" {
		for _, entry := range rule.Metadata {
			if entry.Field == rule.BodyField {
				v.add(prefix+".body_field", "body_field conflicts with metadata field "+entry.Field)
			}
		}
	}
}

var yamlReserved = map[string]bool{
	"true": true, "false": true, "null": true, "yes": true, "no": true,
	"on": true, "off": true, "y": true, "n": true,
}
