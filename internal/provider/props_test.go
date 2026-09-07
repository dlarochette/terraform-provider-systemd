package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dlarochette/terraform-provider-systemd/internal/sdprops"
)

var (
	strType  = tftypes.String
	boolType = tftypes.Bool
	intType  = tftypes.Number
	listStr  = tftypes.List{ElementType: tftypes.String}
)

// objAttrType returns the tftypes type of one directive attribute.
func objAttrType(class string) tftypes.Type {
	switch class {
	case sdprops.ClassBool:
		return boolType
	case sdprops.ClassInt:
		return intType
	case sdprops.ClassList:
		return listStr
	}
	return strType
}

// sectionType builds the tftypes object type of a typed section block.
func sectionType(sec string) tftypes.Object {
	attrs := map[string]tftypes.Type{}
	for _, d := range sdprops.Directives(sec) {
		attrs[d.Attr] = objAttrType(d.Class)
	}
	return tftypes.Object{AttributeTypes: attrs}
}

// fillBlock builds a section object value, nulling attributes not in vals.
func fillBlock(sec string, vals map[string]tftypes.Value) tftypes.Value {
	typ := sectionType(sec)
	attrs := map[string]tftypes.Value{}
	for name := range typ.AttributeTypes {
		if v, ok := vals[name]; ok {
			attrs[name] = v
		} else {
			attrs[name] = nullVal(typ.AttributeTypes[name])
		}
	}
	return tftypes.NewValue(typ, attrs)
}

// renderRawValue builds a raw resource value carrying typed Unit and Service
// blocks.
func renderRawValue() tftypes.Value {
	sections := []string{"Unit", "Install", "Service"}
	attrs := map[string]tftypes.Type{
		"name":    strType,
		"content": strType,
		"enable":  boolType,
		"active":  boolType,
		"id":      strType,
		"section": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"name":  strType,
			"entry": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"key": strType, "value": strType}}},
		}}},
	}
	for _, sec := range sections {
		attrs[strings.ToLower(sec)] = sectionType(sec)
	}
	raw := map[string]tftypes.Value{
		"name":    strVal("demo.service"),
		"content": nullVal(strType),
		"enable":  boolVal(true),
		"active":  nullVal(boolType),
		"id":      nullVal(strType),
		"section": tftypes.NewValue(attrs["section"], nil),
		"unit": fillBlock("Unit", map[string]tftypes.Value{
			"description": strVal("Demo"),
			"requires":    listStrVal("a.service", "b.service"),
		}),
		"service": fillBlock("Service", map[string]tftypes.Value{
			"type":       strVal("oneshot"),
			"exec_start": listStrVal("/bin/true"),
			"restart":    strVal("always"),
		}),
	}
	for _, sec := range sections {
		if _, ok := raw[strings.ToLower(sec)]; !ok {
			raw[strings.ToLower(sec)] = nullVal(sectionType(sec))
		}
	}
	return tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, raw)
}

func strVal(s string) tftypes.Value { return tftypes.NewValue(strType, s) }
func boolVal(b bool) tftypes.Value  { return tftypes.NewValue(boolType, b) }
func nullVal(t tftypes.Type) tftypes.Value {
	return tftypes.NewValue(t, nil)
}
func listStrVal(items ...string) tftypes.Value {
	var elems []tftypes.Value
	for _, i := range items {
		elems = append(elems, strVal(i))
	}
	return tftypes.NewValue(listStr, elems)
}

func TestTypedToFileRender(t *testing.T) {
	raw := renderRawValue()
	f, err := typedToFile(raw, unitSpecs([]string{"Unit", "Install", "Service"}))
	if err != nil {
		t.Fatalf("typedToFile: %v", err)
	}
	rendered := f.Render()
	for _, w := range []string{
		"[Unit]\nDescription=Demo\nRequires=a.service\nRequires=b.service\n",
		"[Service]\nType=oneshot\nExecStart=/bin/true\nRestart=always\n",
	} {
		if !strings.Contains(rendered, w) && !strings.Contains(rendered, strings.ReplaceAll(w, "\n", "\n")+"\n") {
			// Render separates sections with a blank line; check per line.
			for _, line := range strings.Split(strings.TrimSuffix(w, "\n"), "\n") {
				if !strings.Contains(rendered, line) {
					t.Errorf("rendered missing %q in:\n%s", line, rendered)
				}
			}
		}
	}
	if idx := strings.Index(rendered, "[Unit]"); idx == -1 || idx > strings.Index(rendered, "[Service]") {
		t.Errorf("section order wrong:\n%s", rendered)
	}
}

func TestResolveFileContentTyped(t *testing.T) {
	raw := renderRawValue()
	sections := []string{"Unit", "Install", "Service"}
	if _, err := resolveFileContentTyped(types.StringValue("[Unit]\nDescription=x\n"), nil, raw, unitSpecs(sections)); err == nil {
		t.Fatal("expected exclusivity error with content + typed")
	}
	body, err := resolveFileContentTyped(types.StringNull(), nil, raw, unitSpecs(sections))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(body, "Restart=always") {
		t.Errorf("body missing Restart: %s", body)
	}
}

func TestValidateContentSectionsTyped(t *testing.T) {
	sections := []string{"Unit", "Install", "Service"}
	if err := validateContentSectionsTyped(types.StringNull(), nil, renderRawValue(), unitSpecs(sections)); err != nil {
		t.Errorf("typed only should pass: %v", err)
	}
	if err := validateContentSectionsTyped(types.StringValue("x"), nil, renderRawValue(), unitSpecs(sections)); err == nil {
		t.Error("content + typed must fail")
	}
	if err := validateContentSectionsTyped(types.StringNull(), nil, tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{}), unitSpecs(sections)); err == nil {
		t.Error("nothing set must fail")
	}
	if err := validateContentSectionsTyped(types.StringValue("x"), nil, tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{}), unitSpecs(sections)); err != nil {
		t.Errorf("content alone must pass: %v", err)
	}
}

func TestValidateTypedConflicts(t *testing.T) {
	raw := renderRawValue()
	sections := []sectionModel{{
		Name: types.StringValue("Service"),
		Entries: []entryModel{
			{Key: types.StringValue("Environment"), Value: types.StringValue("A=1")},
			{Key: types.StringValue("ExecStart"), Value: types.StringValue("/bin/false")},
		},
	}}
	err := validateTypedConflicts(raw, sections, unitSpecs([]string{"Unit", "Install", "Service"}))
	if err == nil || !strings.Contains(err.Error(), "ExecStart") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestDirectiveValues(t *testing.T) {
	d := sdprops.Directive{Name: "CPUAccounting", Attr: "cpu_accounting", Class: sdprops.ClassBool}
	v, err := directiveValues(d, boolVal(true))
	if err != nil || len(v) != 1 || v[0] != "true" {
		t.Errorf("bool render = %v err %v", v, err)
	}
	d = sdprops.Directive{Name: "Nice", Attr: "nice", Class: sdprops.ClassInt}
	v, err = directiveValues(d, tftypes.NewValue(intType, 10))
	if err != nil || v[0] != "10" {
		t.Errorf("int render = %v err %v", v, err)
	}
	d = sdprops.Directive{Name: "After", Attr: "after", Class: sdprops.ClassList}
	v, err = directiveValues(d, listStrVal("a.target", "b.target"))
	if err != nil || len(v) != 2 {
		t.Errorf("list render = %v err %v", v, err)
	}
	d = sdprops.Directive{Name: "Restart", Attr: "restart", Class: "string"}
	v, err = directiveValues(d, strVal("on-failure"))
	if err != nil || v[0] != "on-failure" {
		t.Errorf("string render = %v err %v", v, err)
	}
}

func TestSetStateContent(t *testing.T) {
	raw := renderRawValue()
	out := setStateContent(raw, "rendered", "demo.service")
	obj, ok := rawObject(out)
	if !ok {
		t.Fatal("not an object")
	}
	if c, ok := rawString(obj["content"]); !ok || c != "rendered" {
		t.Errorf("content = %v", obj["content"])
	}
	if i, ok := rawString(obj["id"]); !ok || i != "demo.service" {
		t.Errorf("id = %v", obj["id"])
	}
	// typed values preserved
	svc, _ := rawObject(obj["service"])
	if r, ok := rawString(svc["restart"]); !ok || r != "always" {
		t.Errorf("service.restart lost: %v", svc["restart"])
	}
}

func TestTypedAttrValidators(t *testing.T) {
	attr := typedAttr("unit", "Service", sdprops.Directive{Name: "Restart", Attr: "restart", Class: "enum:service_restart"})
	sa := attr.(schema.StringAttribute)
	if len(sa.Validators) != 1 {
		t.Error("enum attr must carry a one-of validator")
	}
	attr = typedAttr("unit", "Service", sdprops.Directive{Name: "TimeoutStartSec", Attr: "timeout_start_sec", Class: sdprops.ClassTimespan})
	sa = attr.(schema.StringAttribute)
	if len(sa.Validators) != 1 {
		t.Error("timespan attr must carry a validator")
	}
}

func TestUnitLikeFromRaw(t *testing.T) {
	raw := renderRawValue()
	d, err := unitLikeFromRaw(raw)
	if err != nil {
		t.Fatalf("unitLikeFromRaw: %v", err)
	}
	if d.Name != "demo.service" || !d.HasName {
		t.Errorf("name = %q hasName=%v", d.Name, d.HasName)
	}
	if d.HasContent {
		t.Error("content should be unset")
	}
	if d.Enable == nil || !*d.Enable {
		t.Errorf("enable = %v", d.Enable)
	}
	if d.Active != nil {
		t.Error("active should be nil")
	}
	if len(d.Sections) != 0 {
		t.Errorf("sections = %v", d.Sections)
	}
}

func TestUnitLikeFromRawSections(t *testing.T) {
	attrs := map[string]tftypes.Type{
		"name":    strType,
		"content": strType,
		"enable":  boolType,
		"active":  boolType,
		"id":      strType,
		"section": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"name":  strType,
			"entry": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"key": strType, "value": strType}}},
		}}},
	}
	secType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": strType, "entry": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"key": strType, "value": strType}}}}}
	entryType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"key": strType, "value": strType}}
	sec := tftypes.NewValue(secType, map[string]tftypes.Value{
		"name": strVal("Unit"),
		"entry": tftypes.NewValue(tftypes.List{ElementType: entryType}, []tftypes.Value{
			tftypes.NewValue(entryType, map[string]tftypes.Value{"key": strVal("Description"), "value": strVal("Hi")}),
		}),
	})
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: attrs}, map[string]tftypes.Value{
		"name":    strVal("x.service"),
		"content": nullVal(strType),
		"enable":  nullVal(boolType),
		"active":  nullVal(boolType),
		"id":      nullVal(strType),
		"section": tftypes.NewValue(tftypes.List{ElementType: secType}, []tftypes.Value{sec}),
	})
	d, err := unitLikeFromRaw(raw)
	if err != nil {
		t.Fatalf("unitLikeFromRaw: %v", err)
	}
	if len(d.Sections) != 1 || d.Sections[0].Name.ValueString() != "Unit" {
		t.Fatalf("sections = %+v", d.Sections)
	}
	if len(d.Sections[0].Entries) != 1 {
		t.Fatalf("entries = %+v", d.Sections[0].Entries)
	}
	e := d.Sections[0].Entries[0]
	if e.Key.ValueString() != "Description" || e.Value.ValueString() != "Hi" {
		t.Errorf("entry = %v = %v", e.Key.ValueString(), e.Value.ValueString())
	}
}

func TestValidateTypedForVersion(t *testing.T) {
	// find a directive introduced after v249
	var sec, attr, name string
	var since int
outer:
	for _, s := range sdprops.SectionNames() {
		for _, d := range sdprops.Directives(s) {
			if v := sdprops.SinceVersion(s, d.Name); v > 249 {
				sec, attr, name, since = s, d.Attr, d.Name, v
				break outer
			}
		}
	}
	if name == "" {
		t.Skip("no directive newer than v249 in the catalog")
	}
	raw := renderRawValue()
	obj, _ := rawObject(raw)
	obj[strings.ToLower(sec)] = fillBlock(sec, nil)
	// find the right type and set a plausible value
	for _, d := range sdprops.Directives(sec) {
		if d.Attr == attr {
			var val tftypes.Value
			switch d.Class {
			case sdprops.ClassBool:
				val = boolVal(true)
			case sdprops.ClassInt:
				val = tftypes.NewValue(intType, 1)
			case sdprops.ClassList:
				val = listStrVal("x")
			default:
				val = strVal("1")
			}
			vals := map[string]tftypes.Value{}
			if m, _ := rawObject(obj[strings.ToLower(sec)]); m != nil {
				for k := range m {
					vals[k] = m[k]
				}
			}
			vals[attr] = val
			obj[strings.ToLower(sec)] = fillBlock(sec, vals)
			break
		}
	}
	raw = tftypes.NewValue(raw.Type(), obj)

	// host version below the directive's introduction: rejected
	if err := validateTypedForVersion(raw, unitSpecs([]string{"Unit", "Install", "Service"}), 249); err == nil {
		t.Fatalf("%s.%s (since v%d) must be rejected for v249", sec, name, since)
	}
	// latest version: accepted
	if err := validateTypedForVersion(raw, unitSpecs([]string{"Unit", "Install", "Service"}), sdprops.LatestVersion); err != nil {
		t.Fatalf("latest must accept: %v", err)
	}
}

func TestNetTypedToFileRender(t *testing.T) {
	specs := netSpecs("network")
	// find the Match, Network and Address sections
	objTypes := map[string]tftypes.Type{"filename": strType, "content": strType, "id": strType, "section": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": strType, "entry": tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"key": strType, "value": strType}}}}}}}
	for _, spec := range specs {
		objTypes[blockName(spec.Section)] = netSectionValueType(spec)
	}
	// [Network] block with a couple of typed directives
	networkVals := map[string]tftypes.Value{}
	for _, d := range sdprops.NetDirectives("network", "Network") {
		networkVals[d.Attr] = nullOf(objAttrType2(d.Class))
	}
	if _, ok := networkVals["dns"]; !ok {
		t.Fatal("Network.DNS directive missing from catalog")
	}
	networkVals["dns"] = strVal("9.9.9.9")
	if _, ok := networkVals["dhcp"]; ok {
		networkVals["dhcp"] = strVal("ipv4")
	}
	// [Address] repeatable: two instances
	addrVals := map[string]tftypes.Value{}
	for _, d := range sdprops.NetDirectives("network", "Address") {
		addrVals[d.Attr] = nullOf(objAttrType2(d.Class))
	}
	if _, ok := addrVals["address"]; !ok {
		t.Fatal("Address.Address directive missing")
	}
	inst1 := fillNetBlock("Address", map[string]tftypes.Value{"address": strVal("192.0.2.10/24")})
	inst2 := fillNetBlock("Address", map[string]tftypes.Value{"address": strVal("2001:db8::10/64")})
	rawVals := map[string]tftypes.Value{
		"section":  nullVal(objTypes["section"]),
		"filename": strVal("30-br0.network"),
		"content":  nullVal(strType),
		"id":       nullVal(strType),
		"network":  fillNetBlock("Network", networkVals),
		"address":  tftypes.NewValue(netSectionListType("Address"), []tftypes.Value{inst1, inst2}),
	}
	for _, spec := range specs {
		name := blockName(spec.Section)
		if _, ok := rawVals[name]; !ok {
			rawVals[name] = nullVal(objTypes[name])
		}
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: objTypes}, rawVals)
	f, err := typedToFile(raw, specs)
	if err != nil {
		t.Fatalf("typedToFile: %v", err)
	}
	rendered := f.Render()
	if !strings.Contains(rendered, "DNS=9.9.9.9\n") {
		t.Errorf("missing Network DNS in:\n%s", rendered)
	}
	if strings.Count(rendered, "[Address]") != 2 {
		t.Errorf("expected 2 [Address] sections in:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Address=192.0.2.10/24") || !strings.Contains(rendered, "Address=2001:db8::10/64") {
		t.Errorf("address content wrong:\n%s", rendered)
	}
}

// net helpers

func netSectionValueType(spec sectionSpec) tftypes.Type {
	attrs := map[string]tftypes.Type{}
	for _, d := range spec.directives() {
		attrs[d.Attr] = objAttrType2(d.Class)
	}
	if spec.Repeatable {
		return tftypes.List{ElementType: tftypes.Object{AttributeTypes: attrs}}
	}
	return tftypes.Object{AttributeTypes: attrs}
}

func netSectionListType(sec string) tftypes.Type {
	attrs := map[string]tftypes.Type{}
	for _, d := range sdprops.NetDirectives("network", sec) {
		attrs[d.Attr] = objAttrType2(d.Class)
	}
	return tftypes.List{ElementType: tftypes.Object{AttributeTypes: attrs}}
}

func nullOf(t tftypes.Type) tftypes.Value {
	return tftypes.NewValue(t, nil)
}

func fillNetBlock(sec string, vals map[string]tftypes.Value) tftypes.Value {
	return fillNetBlockKind("network", sec, vals)
}

func fillNetBlockKind(kind, sec string, vals map[string]tftypes.Value) tftypes.Value {
	attrs := map[string]tftypes.Value{}
	typ := tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}
	typ = netSectionSingleType(kind, sec)
	for name := range typ.AttributeTypes {
		if v, ok := vals[name]; ok {
			attrs[name] = v
		} else {
			attrs[name] = nullVal(typ.AttributeTypes[name])
		}
	}
	return tftypes.NewValue(typ, attrs)
}

func netSectionSingleType(kind, sec string) tftypes.Object {
	attrs := map[string]tftypes.Type{}
	for _, d := range sdprops.NetDirectives(kind, sec) {
		attrs[d.Attr] = objAttrType2(d.Class)
	}
	return tftypes.Object{AttributeTypes: attrs}
}

func objAttrType2(class string) tftypes.Type {
	switch class {
	case sdprops.ClassBool:
		return boolType
	case sdprops.ClassInt:
		return intType
	case sdprops.ClassList:
		return listStr
	}
	return strType
}
