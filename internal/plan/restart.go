package plan

func (f *File) ResetProgress() {
	for i := range f.Tasks {
		f.Tasks[i].Status = StatusTodo
		f.Tasks[i].Notes = ""
	}
}
