package provider

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/dlarochette/terraform-provider-systemd/internal/sdprops"
	"github.com/dlarochette/terraform-provider-systemd/internal/unitfile"
)

// ---------------------------------------------------------------- specs

// blockName maps a systemd section name to an HCL block name (lowercase,
// only alphanumerics and underscores).
func blockName(section string) string {
	out := strings.ToLower(section)
	out = strings.ReplaceAll(out, "-", "_")
	out = strings.ReplaceAll(out, ".", "_")
	return out
}

// sectionSpec describes one typed section block: which catalog kind it
// comes from and whether the section may repeat in the file.
type sectionSpec struct {
	Kind       string // "unit", "network", "netdev", "link" or "resolved"
	Section    string
	Repeatable bool
	// Block overrides the HCL block name (used when the default block
	// name would collide with a resource attribute, e.g. `unit`).
	Block string
}

// block returns the HCL block name of the section.
func (s sectionSpec) block() string {
	if s.Block != "" {
		return s.Block
	}
	return blockName(s.Section)
}

func unitSpecs(sections []string) []sectionSpec {
	out := make([]sectionSpec, 0, len(sections))
	for _, sec := range sections {
		out = append(out, sectionSpec{Kind: "unit", Section: sec})
	}
	return out
}

func netSpecs(kind string) []sectionSpec {
	rep := sdprops.RepeatableSections(kind)
	out := make([]sectionSpec, 0)
	for _, sec := range sdprops.NetSectionNames(kind) {
		out = append(out, sectionSpec{Kind: kind, Section: sec, Repeatable: rep[sec]})
	}
	return out
}

func (s sectionSpec) directives() []sdprops.Directive {
	if s.Kind == "unit" || s.Kind == "" {
		return sdprops.Directives(s.Section)
	}
	return sdprops.NetDirectives(s.Kind, s.Section)
}

func (s sectionSpec) sinceVersion(name string) int {
	if s.Kind == "unit" || s.Kind == "" {
		return sdprops.SinceVersion(s.Section, name)
	}
	return sdprops.NetSinceVersion(s.Kind, s.Section, name)
}

// ---------------------------------------------------------------- schema

// typedBlocks builds one single-nested block per systemd section. Each
// block attribute is a systemd directive, named snake_case, typed and
// validated according to the systemd parser.
func typedBlocks(specs []sectionSpec) map[string]schema.Block {
	blocks := map[string]schema.Block{}
	for _, spec := range specs {
		dirs := spec.directives()
		if len(dirs) == 0 {
			continue
		}
		attrs := map[string]schema.Attribute{}
		for _, d := range dirs {
			attrs[d.Attr] = typedAttr(spec.Kind, spec.Section, d)
		}
		desc := fmt.Sprintf(
			"Directives of the systemd `[%s]` section, typed as properties with systemd validation rules. Mutually exclusive with `section` blocks for the directives declared here.",
			spec.Section)
		if spec.Repeatable {
			blocks[spec.block()] = schema.ListNestedBlock{
				MarkdownDescription: desc + " This section may appear several times in the file; repeat this block for each instance.",
				NestedObject: schema.NestedBlockObject{
					Attributes: attrs,
				},
			}
			continue
		}
		blocks[spec.block()] = schema.SingleNestedBlock{
			MarkdownDescription: desc,
			Attributes:          attrs,
		}
	}
	return blocks
}

func typedAttr(kind, section string, d sdprops.Directive) schema.Attribute {
	base := fmt.Sprintf("systemd directive `%s=` in the `[%s]` section (systemd v%d).", d.Name, section, sdprops.LatestVersion)
	switch d.Class {
	case sdprops.ClassBool:
		return schema.BoolAttribute{
			Optional:            true,
			MarkdownDescription: base,
		}
	case sdprops.ClassInt:
		if r, ok := intRange(d.Name); ok {
			return schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: base + fmt.Sprintf(" Range: %d–%d.", r[0], r[1]),
				Validators:          []validator.Int64{intRangeValidator(r[0], r[1])},
			}
		}
		return schema.Int64Attribute{Optional: true, MarkdownDescription: base}
	case sdprops.ClassList:
		return schema.ListAttribute{
			Optional:            true,
			ElementType:         types.StringType,
			MarkdownDescription: base + " Repeatable directive: one value per entry, written one `Key=value` per line.",
		}
	}
	// String classes.
	v := schema.StringAttribute{Optional: true, MarkdownDescription: base}
	switch {
	case sdprops.EnumOf(d.Class) != nil:
		v.Validators = []validator.String{stringOneOf(sdprops.EnumOf(d.Class))}
	case d.Class == sdprops.ClassTimespan:
		v.MarkdownDescription = base + " systemd time span (e.g. `5s`, `10min 30s`, `infinity`)."
		v.Validators = []validator.String{regexValidator(timespanRe, "systemd time span")}
	case d.Class == sdprops.ClassSize:
		v.MarkdownDescription = base + " byte size (e.g. `2G`, `512MiB`), percentage, `infinity` or `max`."
		v.Validators = []validator.String{regexValidator(sizeRe, "systemd size")}
	case d.Class == sdprops.ClassMode:
		v.MarkdownDescription = base + " file mode in octal notation (e.g. `0755`)."
		v.Validators = []validator.String{regexValidator(modeRe, "octal file mode")}
	case d.Class == sdprops.ClassSignal:
		v.MarkdownDescription = base + " signal name (e.g. `SIGTERM`, `SIGHUP`) or signal number."
		v.Validators = []validator.String{regexValidator(signalRe, "unix signal")}
	case d.Class == sdprops.ClassRlimit:
		v.MarkdownDescription = base + " resource limit (`value` or `soft:hard`, `infinity` or `max`)."
		v.Validators = []validator.String{regexValidator(rlimitRe, "resource limit")}
	case d.Class == sdprops.ClassWeight:
		v.MarkdownDescription = base + " weight between 1 and 10000, or `idle`/`max`."
		v.Validators = []validator.String{regexValidator(weightRe, "cgroup weight")}
	case d.Class == sdprops.ClassBlkWeight:
		v.MarkdownDescription = base + " weight between 10 and 1000, or `max`."
		v.Validators = []validator.String{regexValidator(blkWeightRe, "block IO weight")}
	}
	return v
}

// intRange narrows integer directives where systemd enforces a range.
var intRanges = map[string][2]int64{
	"Nice":                  {-20, 19},
	"OOMScoreAdjust":        {-1000, 1000},
	"IOSchedulingPriority":  {0, 7},
	"CPUSchedulingPriority": {0, 99},
	"SwapPriority":          {-1, 32767},
	"CPUShares":             {2, 262144},
}

func intRange(name string) ([2]int64, bool) {
	r, ok := intRanges[name]
	return r, ok
}

var (
	timespanRe  = regexp.MustCompile(`^(?i:infinity)$|^[+-]?(\d+(\.\d+)?(y|M|w|d|h|min|s|ms|us|µs|ns)?)((\s*|\s+)[+-]?(\d+(\.\d+)?(y|M|w|d|h|min|s|ms|us|µs|ns)?))*$`)
	sizeRe      = regexp.MustCompile(`^(\d+(\.\d+)?([KMGTPE]i?B?|B)?|\d+(\.\d+)?%|(?i:infinity|max))$`)
	modeRe      = regexp.MustCompile(`^[0-7]{1,4}$`)
	signalRe    = regexp.MustCompile(`^(?i:(sig)?(hup|int|quit|ill|trap|abrt|bus|fpe|kill|usr1|usr2|segv|pipe|alrm|term|stkflt|chld|cont|stop|tstp|ttin|ttou|urg|xcpu|xfsz|vtalrm|prof|winch|io|pwr|sys|rtmin(\+\d+)?|rtmax)|\d{1,3})$`)
	rlimitRe    = regexp.MustCompile(`^(?i:infinity|max|-?\d+)(:(?i:infinity|max|-?\d+))?$`)
	weightRe    = regexp.MustCompile(`^(?i:idle|max|[1-9][0-9]{0,3}|10000)$`)
	blkWeightRe = regexp.MustCompile(`^(?i:max|10|[1-9][0-9]{2}|1000)$`)
)

// ---------------------------------------------------------------- raw access

func rawObject(v tftypes.Value) (map[string]tftypes.Value, bool) {
	if !v.IsKnown() || v.IsNull() {
		return nil, false
	}
	var m map[string]tftypes.Value
	if err := v.As(&m); err != nil {
		return nil, false
	}
	return m, true
}

func rawString(v tftypes.Value) (string, bool) {
	if !v.IsKnown() || v.IsNull() {
		return "", false
	}
	var s string
	if err := v.As(&s); err != nil {
		return "", false
	}
	return s, true
}

func rawList(v tftypes.Value) ([]tftypes.Value, bool) {
	if !v.IsKnown() || v.IsNull() {
		return nil, false
	}
	var l []tftypes.Value
	if err := v.As(&l); err != nil {
		return nil, false
	}
	return l, true
}

// ---------------------------------------------------------------- rendering

// typedToFile renders the typed section blocks of a raw config/plan value
// into a unit file. Values must be known at this point.
func typedToFile(raw tftypes.Value, specs []sectionSpec) (unitfile.File, error) {
	obj, ok := rawObject(raw)
	if !ok {
		return unitfile.File{}, nil
	}
	var f unitfile.File
	for _, spec := range specs {
		blockVal, present := obj[spec.block()]
		if !present {
			continue
		}
		instances := []map[string]tftypes.Value{}
		if spec.Repeatable {
			elems, ok := rawList(blockVal)
			if !ok {
				continue
			}
			for _, el := range elems {
				if attrs, ok := rawObject(el); ok {
					instances = append(instances, attrs)
				}
			}
		} else if attrs, ok := rawObject(blockVal); ok {
			instances = append(instances, attrs)
		}
		dirs := spec.directives()
		for _, blockAttrs := range instances {
			uSec := unitfile.Section{Name: spec.Section}
			for _, d := range dirs {
				val, present := blockAttrs[d.Attr]
				if !present {
					continue
				}
				svals, err := directiveValues(d, val)
				if err != nil {
					return f, err
				}
				for _, sv := range svals {
					uSec.Entries = append(uSec.Entries, unitfile.Entry{Key: d.Name, Value: sv})
				}
			}
			if len(uSec.Entries) > 0 {
				f.Sections = append(f.Sections, uSec)
			}
		}
	}
	return f, nil
}

func directiveValues(d sdprops.Directive, val tftypes.Value) ([]string, error) {
	if !val.IsKnown() {
		return nil, fmt.Errorf("directive %s.%s has an unknown value; cannot render the unit file", d.Name, d.Attr)
	}
	if val.IsNull() {
		return nil, nil
	}
	switch d.Class {
	case sdprops.ClassBool:
		var b bool
		if err := val.As(&b); err != nil {
			return nil, err
		}
		if b {
			return []string{"true"}, nil
		}
		return []string{"false"}, nil
	case sdprops.ClassInt:
		var n big.Float
		if err := val.As(&n); err != nil {
			return nil, err
		}
		if !n.IsInt() {
			return nil, fmt.Errorf("directive %s must be an integer", d.Name)
		}
		i, _ := n.Int(nil)
		return []string{i.String()}, nil
	case sdprops.ClassList:
		elems, _ := rawList(val)
		out := make([]string, 0, len(elems))
		for _, e := range elems {
			if !e.IsKnown() {
				return nil, fmt.Errorf("directive %s has an unknown list value", d.Name)
			}
			var s string
			if err := e.As(&s); err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	default:
		var s string
		if err := val.As(&s); err != nil {
			return nil, err
		}
		return []string{s}, nil
	}
}

// resolveFileContentTyped renders the final unit body: typed sections first
// (catalog order), then generic `section` blocks, falling back to raw
// `content`.
func resolveFileContentTyped(content types.String, sections []sectionModel, raw tftypes.Value, specs []sectionSpec) (string, error) {
	var structured unitfile.File
	typed, err := typedToFile(raw, specs)
	if err != nil {
		return "", err
	}
	structured.Sections = append(structured.Sections, typed.Sections...)
	structured.Sections = append(structured.Sections, sectionsToFile(sections).Sections...)
	rawBody := ""
	if !content.IsNull() && !content.IsUnknown() {
		rawBody = content.ValueString()
	}
	return unitfile.ChooseContent(rawBody, structured)
}

// ---------------------------------------------------------------- validation

// hasTypedSections reports whether any typed block carries a value.
func hasTypedSections(raw tftypes.Value, specs []sectionSpec) bool {
	obj, ok := rawObject(raw)
	if !ok {
		return false
	}
	for _, spec := range specs {
		blockVal, present := obj[spec.block()]
		if !present {
			continue
		}
		dirs := spec.directives()
		check := func(attrs map[string]tftypes.Value) bool {
			for _, d := range dirs {
				if val, present := attrs[d.Attr]; present && val.IsKnown() && !val.IsNull() {
					return true
				}
			}
			return false
		}
		if spec.Repeatable {
			if elems, ok := rawList(blockVal); ok {
				for _, el := range elems {
					if attrs, ok := rawObject(el); ok && check(attrs) {
						return true
					}
				}
			}
			continue
		}
		if attrs, ok := rawObject(blockVal); ok && check(attrs) {
			return true
		}
	}
	return false
}

// validateContentSectionsTyped enforces the content / section / typed-block
// exclusivity.
func validateContentSectionsTyped(content types.String, sections []sectionModel, raw tftypes.Value, specs []sectionSpec) error {
	hasC := !content.IsNull() && !content.IsUnknown() && content.ValueString() != ""
	hasS := len(sections) > 0
	hasT := hasTypedSections(raw, specs)
	switch {
	case hasC && (hasS || hasT):
		return fmt.Errorf("content is mutually exclusive with section blocks and typed properties; set only one representation")
	case !hasC && !hasS && !hasT:
		return fmt.Errorf("either content, at least one section block, or at least one typed property must be set")
	default:
		return nil
	}
}

// validateTypedConflicts rejects a generic `section` block that sets a
// directive also provided by a typed block of the same section.
func validateTypedConflicts(raw tftypes.Value, sections []sectionModel, specs []sectionSpec) error {
	obj, ok := rawObject(raw)
	if !ok {
		return nil
	}
	for _, s := range sections {
		if s.Name.IsNull() || s.Name.IsUnknown() {
			continue
		}
		secName := s.Name.ValueString()
		for _, spec := range specs {
			if !strings.EqualFold(spec.Section, secName) {
				continue
			}
			blockVal, present := obj[spec.block()]
			if !present {
				continue
			}
			dirs := spec.directives()
			used := map[string]bool{}
			collect := func(attrs map[string]tftypes.Value) {
				for _, d := range dirs {
					if val, present := attrs[d.Attr]; present && val.IsKnown() && !val.IsNull() {
						used[d.Name] = true
					}
				}
			}
			if spec.Repeatable {
				if elems, ok := rawList(blockVal); ok {
					for _, el := range elems {
						if attrs, ok := rawObject(el); ok {
							collect(attrs)
						}
					}
				}
			} else if attrs, ok := rawObject(blockVal); ok {
				collect(attrs)
			}
			for _, e := range s.Entries {
				if used[e.Key.ValueString()] {
					return fmt.Errorf("directive `%s=` is set twice: by the typed property `%s` and by a `section` block; remove one", e.Key.ValueString(), spec.block()+"."+snakeOf(dirs, e.Key.ValueString()))
				}
			}
		}
	}
	return nil
}

func snakeOf(dirs []sdprops.Directive, name string) string {
	for _, d := range dirs {
		if strings.EqualFold(d.Name, name) {
			return d.Attr
		}
	}
	return name
}

// ---------------------------------------------------------------- state

// setStateContent returns the raw value with `content` and `id` set, for
// writing the state without a reflection model.
func setStateContent(raw tftypes.Value, content, id string) tftypes.Value {
	out, err := tftypes.Transform(raw, func(p *tftypes.AttributePath, v tftypes.Value) (tftypes.Value, error) {
		if len(p.Steps()) != 1 {
			return v, nil
		}
		name, ok := p.Steps()[0].(tftypes.AttributeName)
		if !ok {
			return v, nil
		}
		switch string(name) {
		case "content":
			return tftypes.NewValue(tftypes.String, content), nil
		case "id":
			return tftypes.NewValue(tftypes.String, id), nil
		}
		return v, nil
	})
	if err != nil {
		return raw
	}
	return out
}

// ---------------------------------------------------------------- raw model

// unitLikeRaw is the model data of a unit-like resource read from a raw
// tftypes value (the schema has more attributes than any reflection model).
type unitLikeRaw struct {
	Name       string
	HasName    bool
	Content    string
	HasContent bool
	Sections   []sectionModel
	Enable     *bool
	Active     *bool
}

func unitLikeFromRaw(raw tftypes.Value) (unitLikeRaw, error) {
	var d unitLikeRaw
	obj, ok := rawObject(raw)
	if !ok {
		return d, fmt.Errorf("config is not an object")
	}
	if s, ok := rawString(obj["name"]); ok {
		d.Name = s
		d.HasName = true
	}
	if s, ok := rawString(obj["content"]); ok {
		d.Content = s
		d.HasContent = true
	}
	if b, ok := rawOptBool(obj["enable"]); ok {
		d.Enable = b
	}
	if b, ok := rawOptBool(obj["active"]); ok {
		d.Active = b
	}
	if entries, ok := rawSectionBlocks(obj["section"]); ok {
		d.Sections = entries
	}
	return d, nil
}

func mustStr(v tftypes.Value) string {
	s, _ := rawString(v)
	return s
}

func rawSectionBlocks(v tftypes.Value) ([]sectionModel, bool) {
	elems, ok := rawList(v)
	if !ok {
		return nil, false
	}
	var out []sectionModel
	for _, el := range elems {
		eAttrs, ok := rawObject(el)
		if !ok {
			continue
		}
		sec := sectionModel{Name: types.StringValue(mustStr(eAttrs["name"]))}
		if entryElems, ok := rawList(eAttrs["entry"]); ok {
			for _, en := range entryElems {
				enAttrs, ok := rawObject(en)
				if !ok {
					continue
				}
				sec.Entries = append(sec.Entries, entryModel{
					Key:   types.StringValue(mustStr(enAttrs["key"])),
					Value: types.StringValue(mustStr(enAttrs["value"])),
				})
			}
		}
		out = append(out, sec)
	}
	return out, false || ok
}

func rawOptBool(v tftypes.Value) (*bool, bool) {
	if !v.IsKnown() || v.IsNull() {
		return nil, false
	}
	var b bool
	if err := v.As(&b); err != nil {
		return nil, false
	}
	return &b, true
}

// validateTypedForVersion checks that every directive set in a typed block
// exists in the given systemd release of the host.
func validateTypedForVersion(raw tftypes.Value, specs []sectionSpec, version int) error {
	obj, ok := rawObject(raw)
	if !ok {
		return nil
	}
	var missing []string
	for _, spec := range specs {
		blockVal, present := obj[spec.block()]
		if !present {
			continue
		}
		dirs := spec.directives()
		if spec.Repeatable {
			if elems, ok := rawList(blockVal); ok {
				for _, el := range elems {
					if attrs, ok := rawObject(el); ok {
						missing = appendInstanceMissing(spec, dirs, attrs, version, missing)
					}
				}
			}
		} else if attrs, ok := rawObject(blockVal); ok {
			missing = appendInstanceMissing(spec, dirs, attrs, version, missing)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("directives not available in systemd v%d of the host:\n%s", version, strings.Join(missing, "\n"))
	}
	return nil
}

func appendInstanceMissing(spec sectionSpec, dirs []sdprops.Directive, attrs map[string]tftypes.Value, version int, missing []string) []string {
	for _, d := range dirs {
		val, present := attrs[d.Attr]
		if !present || !val.IsKnown() || val.IsNull() {
			continue
		}
		if since := spec.sinceVersion(d.Name); since > version {
			missing = append(missing, fmt.Sprintf("`[%s]` `%s=` requires systemd >= v%d (host: v%d)", spec.Section, d.Name, since, version))
		}
	}
	return missing
}

// networkFileRaw is the model data of a network file resource read from a
// raw tftypes value.
type networkFileRaw struct {
	Filename    string
	HasFilename bool
	Content     string
	HasContent  bool
	Sections    []sectionModel
}

func networkFileFromRaw(raw tftypes.Value) (networkFileRaw, error) {
	var d networkFileRaw
	obj, ok := rawObject(raw)
	if !ok {
		return d, fmt.Errorf("config is not an object")
	}
	if s, ok := rawString(obj["filename"]); ok {
		d.Filename = s
		d.HasFilename = true
	}
	if s, ok := rawString(obj["content"]); ok {
		d.Content = s
		d.HasContent = true
	}
	if secs, ok := rawSectionBlocks(obj["section"]); ok {
		d.Sections = secs
	}
	return d, nil
}

// setStateContentFilename transforms a raw value setting content and id for
// network file resources (id = filename).
func setStateContentFilename(raw tftypes.Value, content, filename string) tftypes.Value {
	return setStateContent(raw, content, filename)
}

// resolvedConfRaw is the model data of resolved.conf resources read from a
// raw tftypes value (content only, no name).
type resolvedConfRaw struct {
	Content    string
	HasContent bool
	Sections   []sectionModel
}

func resolvedConfFromRaw(raw tftypes.Value) (resolvedConfRaw, error) {
	var d resolvedConfRaw
	obj, ok := rawObject(raw)
	if !ok {
		return d, fmt.Errorf("config is not an object")
	}
	if s, ok := rawString(obj["content"]); ok {
		d.Content = s
		d.HasContent = true
	}
	if secs, ok := rawSectionBlocks(obj["section"]); ok {
		d.Sections = secs
	}
	return d, nil
}

// dropinRaw is the model data of a systemd drop-in resource read from a
// raw tftypes value.
type dropinRaw struct {
	Unit       string
	Dropin     string
	HasUnit    bool
	HasDropin  bool
	Content    string
	HasContent bool
	Sections   []sectionModel
}

func dropinFromRaw(raw tftypes.Value) (dropinRaw, error) {
	var d dropinRaw
	obj, ok := rawObject(raw)
	if !ok {
		return d, fmt.Errorf("config is not an object")
	}
	if s, ok := rawString(obj["unit"]); ok {
		d.Unit = s
		d.HasUnit = true
	}
	if s, ok := rawString(obj["dropin"]); ok {
		d.Dropin = s
		d.HasDropin = true
	}
	if s, ok := rawString(obj["content"]); ok {
		d.Content = s
		d.HasContent = true
	}
	if secs, ok := rawSectionBlocks(obj["section"]); ok {
		d.Sections = secs
	}
	return d, nil
}
