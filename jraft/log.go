package jraft

//日志条目，我们这里简单假定command是以key=value形式
type entry struct {
	key   string
	value interface{}
	term  int //entry属于的任期
}
type raftlog struct {
	entries  []entry
	index    int //日志索引
	commited int //上次提交的位置
}
