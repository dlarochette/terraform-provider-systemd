package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dlarochette/terraform-provider-systemd/internal/unitfile"
)

// sectionModel is a systemd INI section in HCL.
type sectionModel struct {
	Name    types.String `tfsdk:"name"`
	Entries []entryModel `tfsdk:"entry"`
}

type entryModel struct {
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
}

func sectionBlockSchema() schema.ListNestedBlock {
	return schema.ListNestedBlock{
		MarkdownDescription: "Structured systemd INI sections. Mutually exclusive with `content`.",
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{
					Required:            true,
					MarkdownDescription: "Section name without brackets (e.g. `Unit`, `Service`, `Network`).",
					Validators: []validator.String{
						nonEmptyContent(),
					},
				},
			},
			Blocks: map[string]schema.Block{
				"entry": schema.ListNestedBlock{
					MarkdownDescription: "Key/value entries. Duplicate keys are allowed (systemd multi-value).",
					NestedObject: schema.NestedBlockObject{
						Attributes: map[string]schema.Attribute{
							"key": schema.StringAttribute{
								Required: true,
								Validators: []validator.String{
									nonEmptyContent(),
								},
							},
							"value": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "Value as written after `=` (may be empty).",
							},
						},
					},
				},
			},
		},
	}
}

// optionalContentAttribute is raw file body; exclusive with section blocks.
func optionalContentAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: "Raw file contents. Mutually exclusive with `section` blocks. When using `section`, this is computed from the rendered INI.",
	}
}

func sectionsToFile(sections []sectionModel) unitfile.File {
	var f unitfile.File
	for _, s := range sections {
		sec := unitfile.Section{Name: s.Name.ValueString()}
		for _, e := range s.Entries {
			sec.Entries = append(sec.Entries, unitfile.Entry{
				Key:   e.Key.ValueString(),
				Value: e.Value.ValueString(),
			})
		}
		f.Sections = append(f.Sections, sec)
	}
	return f
}

func resolveFileContent(content types.String, sections []sectionModel) (string, error) {
	raw := ""
	if !content.IsNull() && !content.IsUnknown() {
		raw = content.ValueString()
	}
	// Config may leave content unknown/null when only sections are set.
	if content.IsUnknown() {
		raw = ""
	}
	return unitfile.ChooseContent(raw, sectionsToFile(sections))
}

func validateContentOrSections(content types.String, sections []sectionModel) error {
	hasC := !content.IsNull() && !content.IsUnknown() && content.ValueString() != ""
	hasS := len(sections) > 0
	switch {
	case hasC && hasS:
		return fmt.Errorf("content and section are mutually exclusive; set only one")
	case !hasC && !hasS:
		return fmt.Errorf("either content or at least one section block must be set")
	default:
		return nil
	}
}
