package main

import (
	"container/list"
	"math/rand"
	"testing"
)

// https://stackoverflow.com/a/69187698

// wanna compare
// - map insertions
// - slice insertions
// - containers/list insertions
// - custom linked list insertions

// this basically proves that slices are better
// -> even though theoretically a linked list is the perfect data structure for fastest appends
// -> in practice, pre-allocated slices are the best bet

type Data struct {
	time int64
	lat  float32
	lon  float32
}

func RandData(i int64) Data {
	return Data{
		i,
		rand.Float32()*180 - 90,
		rand.Float32()*360 - 180,
	}
}

func BenchmarkMap(b *testing.B) {
	var c map[int64]Data = map[int64]Data{}
	for i := int64(0); i < int64(b.N); i++ {
		c[i] = RandData(i)
	}
}

func BenchmarkSlice(b *testing.B) {
	var c []Data = []Data{}
	for i := int64(0); i < int64(b.N); i++ {
		//lint:ignore SA4010 we're just doing this for testing
		c = append(c, RandData(i))
	}
}

func BenchmarkList(b *testing.B) {
	var c *list.List = list.New()
	for i := int64(0); i < int64(b.N); i++ {
		c.PushBack(RandData(i))
	}
}

func BenchmarkDataList(b *testing.B) {
	var c *DataList = New()
	for i := int64(0); i < int64(b.N); i++ {
		c.PushBack(RandData(i))
	}
}

// ripped-off from containers/list

type Element struct {
	next, prev *Element
	Value      Data
}
type DataList struct {
	root Element
	len  int
}

func (l *DataList) Init() *DataList {
	l.root.next = &l.root
	l.root.prev = &l.root
	l.len = 0
	return l
}

// func (l *DataList) lazyInit() {
// 	if l.root.next == nil {
// 		l.Init()
// 	}
// }

func New() *DataList         { return new(DataList).Init() }
func (l *DataList) Len() int { return l.len }
func (l *DataList) PushBack(v Data) *Element {
	// l.lazyInit()
	return l.insertValue(v, l.root.prev)
}
func (l *DataList) insertValue(v Data, at *Element) *Element {
	return l.insert(&Element{Value: v}, at)
}
func (l *DataList) insert(e, at *Element) *Element {
	e.prev = at
	e.next = at.next
	e.prev.next = e
	e.next.prev = e
	// e.list = l
	l.len++
	return e
}
