package wasm

import (
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/internalapi"
)

// ImportedTables implements the same method as documented on wazero.CompiledModule.
func (m *Module) ImportedTables() (ret []api.TableDefinition) {
	for i := range m.TableDefinitionSection {
		d := &m.TableDefinitionSection[i]
		if d.importDesc != nil {
			ret = append(ret, d)
		}
	}
	return
}

// ExportedTables implements the same method as documented on wazero.CompiledModule.
func (m *Module) ExportedTables() map[string]api.TableDefinition {
	ret := map[string]api.TableDefinition{}
	for i := range m.TableDefinitionSection {
		d := &m.TableDefinitionSection[i]
		for _, e := range d.exportNames {
			ret[e] = d
		}
	}
	return ret
}

// BuildTableDefinitions generates table metadata that can be parsed from
// the module, mirroring BuildMemoryDefinitions. This must be called after
// all validation.
//
// Note: This is exported for wazero.Runtime CompileModule.
func (m *Module) BuildTableDefinitions() {
	var moduleName string
	if m.NameSection != nil {
		moduleName = m.NameSection.ModuleName
	}

	tableCount := m.ImportTableCount + uint32(len(m.TableSection))
	if tableCount == 0 {
		return
	}

	m.TableDefinitionSection = make([]TableDefinition, 0, tableCount)
	importTableIdx := Index(0)
	for i := range m.ImportSection {
		imp := &m.ImportSection[i]
		if imp.Type != ExternTypeTable {
			continue
		}

		desc := imp.DescTable
		m.TableDefinitionSection = append(m.TableDefinitionSection, TableDefinition{
			importDesc: &[2]string{imp.Module, imp.Name},
			index:      importTableIdx,
			table:      &desc,
		})
		importTableIdx++
	}

	for i := range m.TableSection {
		m.TableDefinitionSection = append(m.TableDefinitionSection, TableDefinition{
			index: importTableIdx,
			table: &m.TableSection[i],
		})
		importTableIdx++
	}

	for i := range m.TableDefinitionSection {
		d := &m.TableDefinitionSection[i]
		d.moduleName = moduleName
		for i := range m.ExportSection {
			e := &m.ExportSection[i]
			if e.Type == ExternTypeTable && e.Index == d.index {
				d.exportNames = append(d.exportNames, e.Name)
			}
		}
	}
}

// TableDefinition implements api.TableDefinition
type TableDefinition struct {
	internalapi.WazeroOnlyType
	moduleName  string
	index       Index
	importDesc  *[2]string
	exportNames []string
	table       *Table
}

// ModuleName implements the same method as documented on api.TableDefinition.
func (d *TableDefinition) ModuleName() string {
	return d.moduleName
}

// Index implements the same method as documented on api.TableDefinition.
func (d *TableDefinition) Index() uint32 {
	return d.index
}

// Import implements the same method as documented on api.TableDefinition.
func (d *TableDefinition) Import() (moduleName, name string, isImport bool) {
	if importDesc := d.importDesc; importDesc != nil {
		moduleName, name, isImport = importDesc[0], importDesc[1], true
	}
	return
}

// ExportNames implements the same method as documented on api.TableDefinition.
func (d *TableDefinition) ExportNames() []string {
	return d.exportNames
}

// Type implements the same method as documented on api.TableDefinition.
func (d *TableDefinition) Type() api.ValueType {
	return d.table.Type.Kind()
}

// Min implements the same method as documented on api.TableDefinition.
func (d *TableDefinition) Min() uint32 {
	return d.table.Min
}

// Max implements the same method as documented on api.TableDefinition.
func (d *TableDefinition) Max() (max uint32, ok bool) {
	if d.table.Max == nil {
		return 0, false
	}
	return *d.table.Max, true
}
