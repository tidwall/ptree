// Copyright 2021 Joshua J Baker. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ptree

import (
	"math"
	"math/rand"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/tidwall/geoindex"
	"github.com/tidwall/geoindex/child"
	"github.com/tidwall/lotsa"
)

func init() {
	seed := time.Now().UnixNano()
	println("seed:", seed)
	rand.Seed(seed)
}

func randPoints(N int, min, max [2]float64) [][2]float64 {
	var points [][2]float64
	var w = max[0] - min[0]
	var h = max[1] - min[1]
	for i := 0; i < N; i++ {
		points = append(points, [2]float64{
			rand.Float64()*w - w/2,
			rand.Float64()*h - h/2,
		})
	}
	return points
}

func boundsForPoints(points [][2]float64) (min, max [2]float64) {
	if len(points) > 0 {
		min[0] = math.Inf(+1)
		min[1] = math.Inf(+1)
		max[0] = math.Inf(-1)
		max[1] = math.Inf(-1)
		for _, p := range points {
			min, max = expand(min, max, p, p)
		}
	}
	return min, max
}

func TestPTree(t *testing.T) {
	N := 10_000
	points := randPoints(N, [2]float64{-180, -90}, [2]float64{180, 90})
	tr := New([2]float64{-180, -90}, [2]float64{180, 90})
	var bmin, bmax [2]float64

	var testChildren = func(nPoints int) {
		var citems []int
		mitems := make(map[int]bool)
		var cnodes []interface{}
		cnodes = append(cnodes, nil)
		for len(cnodes) > 0 {
			cnode := cnodes[len(cnodes)-1]
			cnodes = cnodes[:len(cnodes)-1]
			children := tr.Children(cnode, nil)
			for _, child := range children {
				if child.Item {
					citems = append(citems, child.Data.(int))
					mitems[child.Data.(int)] = true
				} else {
					cnodes = append(cnodes, child.Data)
				}
			}
		}
		if len(citems) != len(mitems) || len(citems) != nPoints {
			t.Fatal("!")
		}
	}

	for i := 0; i < len(points); i++ {
		if i == 0 {
			bmin, bmax = points[i], points[i]
		} else {
			bmin, bmax = expand(bmin, bmax, points[i], points[i])
		}
		tr.Insert(points[i], i)
		if tr.Len() != i+1 {
			t.Fatalf("expected %d, got %d", i+1, tr.Len())
		}
		tmin, tmax := tr.MinBounds()
		if tmin != bmin || tmax != bmax {
			t.Fatalf("expected '%v,%v', got '%v,%v'", bmin, bmax, tmin, tmax)
		}
		var count int
		tr.Search(points[i], points[i],
			func(point [2]float64, data interface{}) bool {
				if point == points[i] && data.(int) == i {
					count++
				}
				return true
			},
		)
		if count != 1 {
			t.Fatalf("expected %d, got %d", 1, count)
		}
		if i&63 == 0 {
			testSearch(t, tr, points[:tr.Len()], 0)
			testChildren(i + 1)
		}
	}

	// scan test
	var count int
	tr.Scan(func(point [2]float64, data interface{}) bool {
		i := data.(int)
		if point == points[i] && data.(int) == i {
			count++
		}
		return true
	})
	if count != len(points) {
		t.Fatalf("expected %d, got %d", len(points), count)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			testSearch(t, tr, points, 0)
		}()
	}
	wg.Wait()
	for i := 0; i < len(points); i++ {
		if i&63 == 0 {
			testSearch(t, tr, points[i:], i)
			testChildren(len(points[i:]))
		}
		bmin, bmax := boundsForPoints(points[i:])
		tmin, tmax := tr.MinBounds()
		if tmin != bmin || tmax != bmax {
			t.Fatalf("expected '%v,%v', got '%v,%v'", bmin, bmax, tmin, tmax)
		}
		tr.Delete(points[i], i)
		if tr.Len() != len(points)-i-1 {
			t.Fatalf("expected %d, got %d", len(points)-i-1, tr.Len())
		}
		var count int
		tr.Search(points[i], points[i],
			func(point [2]float64, data interface{}) bool {
				if point == points[i] && data.(int) == i {
					count++
				}
				return true
			},
		)
		if count != 0 {
			t.Fatalf("expected %d, got %d", 0, count)
		}
	}
	bmin, bmax = boundsForPoints(points[:0])
	tmin, tmax := tr.MinBounds()
	if tmin != bmin || tmax != bmax {
		t.Fatalf("expected '%v,%v', got '%v,%v'", bmin, bmax, tmin, tmax)
	}

}

type trwrap struct{ tr *PTree }

var _ geoindex.Interface = &trwrap{}

func (tr *trwrap) Insert(min, max [2]float64, data interface{}) {
	tr.tr.Insert(min, data)
}

// Delete an item from the structure
func (tr *trwrap) Delete(min, max [2]float64, data interface{}) {
	tr.tr.Delete(min, data)
}

// Replace an item in the structure. This is effectively just a Delete
// followed by an Insert. But for some structures it may be possible to
// optimize the operation to avoid multiple passes
func (tr *trwrap) Replace(
	oldMin, oldMax [2]float64, oldData interface{},
	newMin, newMax [2]float64, newData interface{},
) {
	tr.tr.Delete(oldMin, oldData)
	tr.tr.Insert(newMin, newData)
}

// Search the structure for items that intersects the rect param
func (tr *trwrap) Search(
	min, max [2]float64,
	iter func(min, max [2]float64, data interface{}) bool,
) {
	tr.tr.Search(min, max, func(point [2]float64, data interface{}) bool {
		return iter(point, point, data)
	})
}

// Scan iterates through all data in tree in no specified order.
func (tr *trwrap) Scan(iter func(min, max [2]float64, data interface{}) bool) {
	tr.tr.Scan(func(point [2]float64, data interface{}) bool {
		return iter(point, point, data)
	})

}
func (tr *trwrap) Len() int                      { return tr.Len() }
func (tr *trwrap) Bounds() (min, max [2]float64) { return tr.Bounds() }
func (tr *trwrap) Children(parent interface{}, reuse []child.Child,
) (children []child.Child) {
	return tr.tr.Children(parent, reuse)
}

func TestCitiesSVG(t *testing.T) {
	tr := New([2]float64{-180, -90}, [2]float64{180, 90})
	geoindex.Tests.TestCitiesSVG(t, &trwrap{tr})
}

type searchResult struct {
	point [2]float64
	data  interface{}
}

func testSearch(t *testing.T, tr *PTree, points [][2]float64, sidx int) {
	min := [2]float64{
		rand.Float64()*400 - 200,
		rand.Float64()*200 - 100,
	}
	max := [2]float64{
		rand.Float64()*400 - 200,
		rand.Float64()*200 - 100,
	}
	if min[0] > max[0] {
		min[0], max[0] = max[0], min[0]
	}
	if min[1] > max[1] {
		min[1], max[1] = max[1], min[1]
	}
	var res1 []searchResult
	tr.Search(min, max, func(point [2]float64, data interface{}) bool {
		res1 = append(res1, searchResult{point, data})
		return true
	})

	var res2 []searchResult
	for i := 0; i < len(points); i++ {
		if contains(min, max, points[i]) {
			res2 = append(res2, searchResult{points[i], i + sidx})
		}
	}
	sort.Slice(res1, func(i, j int) bool {
		return res1[i].data.(int) < res1[j].data.(int)
	})
	sort.Slice(res2, func(i, j int) bool {
		return res2[i].data.(int) < res2[j].data.(int)
	})

	if len(res1) != len(res2) {
		t.Fatal("mismatch")
	}
	for i := 0; i < len(res1); i++ {
		if res1[i] != res2[i] {
			t.Fatal("mismatch")
		}
	}

}

func TestBench(t *testing.T) {
	N := 1_000_000
	points := randPoints(N, [2]float64{-180, -90}, [2]float64{180, 90})
	tr := New([2]float64{-180, -90}, [2]float64{180, 90})
	lotsa.Output = os.Stdout
	print("insert:  ")
	lotsa.Ops(N, 1, func(i, _ int) {
		tr.Insert(points[i], i)
	})
	print("search:  ")
	lotsa.Ops(N, 1, func(i, _ int) {
		var found bool
		tr.Search(points[i], points[i],
			func(point [2]float64, data interface{}) bool {
				if data.(int) == i {
					found = true
					return false
				}
				return true
			},
		)
		if !found {
			t.Fatal("not found")
		}
	})
	print("delete:  ")
	lotsa.Ops(N, 1, func(i, _ int) {
		tr.Delete(points[i], i)
	})
}
