package internalwasm

func (m *Module) buildTable() *TableInstance {
	table := m.TableSection
	if table != nil {
		return &TableInstance{
			Table: make([]uintptr, table.Min),
			Min:   table.Min,
			Max:   table.Max,
		}
	}
	return nil
}
