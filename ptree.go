// Copyright 2021 Joshua J Baker. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package ptree

import (
	"math"

	"github.com/tidwall/geoindex/child"
)

const maxEntries = 256                   // max number of entries per node
const minEntries = maxEntries * 40 / 100 // min number of entries per node

const maxHeight = 16 // a limit is needed to avoid infinite splits
const rows = 16      // 16 = 256 child nodes, 8 = 64, 4 = 16, 2 = 4

type item struct {
	point [2]float64
	data  interface{}
}

type node struct {
	nodes *[rows * rows]*node
	count int
	items []item
}

// PTree is a tree for storing points.
type PTree struct {
	min  [2]float64
	max  [2]float64
	root node
}

// New returns a new PTree with the provided maximum bounding rectangle.
func New(min, max [2]float64) *PTree {
	return &PTree{min: min, max: max}
}

// InBounds return true if the point can be contained in the tree's maximum
// bounding rectangle.
func (tr *PTree) InBounds(point [2]float64) bool {
	return contains(tr.min, tr.max, point)
}

// Insert a point into the tree.
func (tr *PTree) Insert(point [2]float64, data interface{}) {
	if !tr.InBounds(point) {
		panic("point out of bounds")
	}
	tr.root.insert(tr.min, tr.max, point, data, 1)
}

func (n *node) split(nmin, nmax [2]float64, depth int) {
	n.nodes = new([rows * rows]*node)
	n.count = 0
	for _, item := range n.items {
		n.insert(nmin, nmax, item.point, item.data, depth)
	}
	n.items = nil
}

func contains(min, max, pt [2]float64) bool {
	return !(pt[0] < min[0] || pt[0] > max[0] ||
		pt[1] < min[1] || pt[1] > max[1])
}

// bottom-up z-order
func calcNodeIndex(x, y int) int {
	return y*rows + x
}

func fmin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func fmax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (n *node) insert(nmin, nmax, point [2]float64, data interface{}, depth int,
) {
	if n.nodes == nil {
		if len(n.items) < maxEntries || depth > maxHeight {
			n.items = append(n.items, item{point: point, data: data})
			n.count++
			return
		}
		n.split(nmin, nmax, depth)
	}

	// choose the coordinates of the child node to insert into
	cx := int((point[0] - nmin[0]) / (nmax[0] - nmin[0]) * rows) // node x index
	cy := int((point[1] - nmin[1]) / (nmax[1] - nmin[1]) * rows) // node y index

	cidx, cmin, cmax := n.getChildNodeIndex(nmin, nmax, cx, cy)

	// insert into the node
	if n.nodes[cidx] == nil {
		n.nodes[cidx] = new(node)
	}
	n.nodes[cidx].insert(cmin, cmax, point, data, depth+1)
	n.count++
}

// Search for points in the tree that are within the provided rectangle.
func (tr *PTree) Search(min, max [2]float64,
	iter func(point [2]float64, data interface{}) bool,
) {
	tr.root.search(tr.min, tr.max, min, max, iter)
}

func (n *node) search(
	nmin, nmax [2]float64, // node rectangle
	smin, smax [2]float64, // search rectangle
	iter func(point [2]float64, data interface{}) bool,
) bool {
	if n.nodes == nil {
		for _, item := range n.items {
			if contains(smin, smax, item.point) {
				if !iter(item.point, item.data) {
					return false
				}
			}
		}
		return true
	}

	// clip the search rectangle
	smin[0] = fmax(smin[0], nmin[0])
	smin[1] = fmax(smin[1], nmin[1])
	smax[0] = fmin(smax[0], nmax[0])
	smax[1] = fmin(smax[1], nmax[1])

	// choose the coordinates of the child node to search
	cx1 := int((smin[0] - nmin[0]) / (nmax[0] - nmin[0]) * rows) // x min index
	cy1 := int((smin[1] - nmin[1]) / (nmax[1] - nmin[1]) * rows) // y min index
	cx2 := int((smax[0] - nmin[0]) / (nmax[0] - nmin[0]) * rows) // x max index
	cy2 := int((smax[1] - nmin[1]) / (nmax[1] - nmin[1]) * rows) // y max index

	// clip the max boundaries of the coordinates
	if cx2 >= rows {
		cx2 = rows - 1
	}
	if cy2 >= rows {
		cy2 = rows - 1
	}

	// scan over all child nodes within the coordinates range
	for cy := cy1; cy <= cy2; cy++ {
		for cx := cx1; cx <= cx2; cx++ {
			cidx, cmin, cmax := n.getChildNodeIndex(nmin, nmax, cx, cy)
			cn := n.nodes[cidx]
			if cn != nil {
				if !cn.search(cmin, cmax, smin, smax, iter) {
					return false
				}
			}
		}
	}
	return true
}

// Delete a point for the tree
func (tr *PTree) Delete(point [2]float64, data interface{}) {
	tr.root.delete(tr.min, tr.max, point, data)
}

func (n *node) delete(nmin, nmax, point [2]float64, data interface{}) bool {
	if n.nodes == nil {
		for i := 0; i < len(n.items); i++ {
			if n.items[i].point == point && n.items[i].data == data {
				n.items[i] = n.items[len(n.items)-1]
				n.items[len(n.items)-1].data = nil
				n.items = n.items[:len(n.items)-1]
				n.count--
				return true
			}
		}
		return false
	}

	// choose the coordinates of the child node to delete from
	cx := int((point[0] - nmin[0]) / (nmax[0] - nmin[0]) * rows) // node x index
	cy := int((point[1] - nmin[1]) / (nmax[1] - nmin[1]) * rows) // node y index

	cidx, cmin, cmax := n.getChildNodeIndex(nmin, nmax, cx, cy)

	cn := n.nodes[cidx]
	if cn != nil {
		// delete from the node
		if !cn.delete(cmin, cmax, point, data) {
			return false
		}
		if cn.count == 0 {
			n.nodes[cidx] = nil
		}
	}
	n.count--
	if n.count < minEntries {
		// compact the node
		var items []item
		n.items = n.gather(items)
		n.nodes = nil
	}
	return true
}

func (n *node) gather(items []item) []item {
	items = append(items, n.items...)
	if n.nodes != nil {
		for i := 0; i < rows*rows; i++ {
			if n.nodes[i] != nil {
				items = n.nodes[i].gather(items)
			}
		}
	}
	return items
}

// Len returns the number of points in the tree
func (tr *PTree) Len() int {
	return tr.root.count
}

// Scan all items in tree
func (tr *PTree) Scan(iter func(point [2]float64, data interface{}) bool) {
	tr.root.scan(iter)
}

func (n *node) scan(iter func(point [2]float64, data interface{}) bool) bool {
	if n.nodes == nil {
		for i := 0; i < len(n.items); i++ {
			if !iter(n.items[i].point, n.items[i].data) {
				return false
			}
		}
	} else {
		for i := 0; i < len(n.nodes); i++ {
			if n.nodes[i].count > 0 {
				if !n.nodes[i].scan(iter) {
					return false
				}
			}
		}
	}
	return true
}

func expand(amin, amax, bmin, bmax [2]float64) (min, max [2]float64) {
	if bmin[0] < amin[0] {
		amin[0] = bmin[0]
	}
	if bmax[0] > amax[0] {
		amax[0] = bmax[0]
	}
	if bmin[1] < amin[1] {
		amin[1] = bmin[1]
	}
	if bmax[1] > amax[1] {
		amax[1] = bmax[1]
	}
	return amin, amax
}

// MinBounds returns the minumum bounding rectangle of the tree.
func (tr *PTree) MinBounds() (min, max [2]float64) {
	if tr.Len() == 0 {
		return
	}
	min[0] = tr.root.minValue(0, math.Inf(+1))
	min[1] = tr.root.minValue(1, math.Inf(+1))
	max[0] = tr.root.maxValue(0, math.Inf(-1))
	max[1] = tr.root.maxValue(1, math.Inf(-1))
	return min, max
}

func (n *node) minValue(coord int, value float64) float64 {
	if n.nodes == nil {
		for _, item := range n.items {
			if item.point[coord] < value {
				value = item.point[coord]
			}
		}
	} else {
		for ci := 0; ci < rows; ci++ {
			for cj := 0; cj < rows; cj++ {
				cx, cy := ci, cj
				if coord == 1 {
					cx, cy = cy, cx
				}
				cn := n.nodes[calcNodeIndex(cx, cy)]
				if cn != nil {
					value = cn.minValue(coord, value)
				}
			}
			if !math.IsInf(value, 0) {
				break
			}
		}
	}
	return value
}

func (n *node) maxValue(coord int, value float64) float64 {
	if n.nodes == nil {
		for _, item := range n.items {
			if item.point[coord] > value {
				value = item.point[coord]
			}
		}
	} else {
		for ci := rows - 1; ci >= 0; ci-- {
			for cj := rows - 1; cj >= 0; cj-- {
				cx, cy := ci, cj
				if coord == 1 {
					cx, cy = cy, cx
				}
				cn := n.nodes[calcNodeIndex(cx, cy)]
				if cn != nil {
					value = cn.maxValue(coord, value)
				}
			}
			if !math.IsInf(value, 0) {
				break
			}
		}
	}
	return value
}

type childNode struct {
	min, max [2]float64
	node     *node
}

// Children returns all children for parent node. If parent node is nil
// then the root nodes should be returned.
// The reuse buffer is an empty length slice that can optionally be used
// to avoid extra allocations.
func (tr *PTree) Children(parent interface{}, reuse []child.Child,
) (children []child.Child) {
	children = reuse[:0]
	var nmin, nmax [2]float64
	var n *node
	if parent == nil {
		children = append(children, child.Child{
			Min: tr.min, Max: tr.max,
			Data: childNode{tr.min, tr.max, &tr.root},
			Item: false,
		})
		return children
	}
	cnode := parent.(childNode)
	nmin, nmax = cnode.min, cnode.max
	n = cnode.node
	if n.nodes == nil {
		// scan over child items
		for _, item := range n.items {
			children = append(children, child.Child{
				Min: item.point, Max: item.point,
				Data: item.data, Item: true,
			})
		}
	} else {
		// scan over all child nodes
		for cy := 0; cy < rows; cy++ {
			for cx := 0; cx < rows; cx++ {
				cidx, cmin, cmax := n.getChildNodeIndex(nmin, nmax, cx, cy)
				cn := n.nodes[cidx]
				if cn == nil || cn.count == 0 {
					continue
				}
				children = append(children, child.Child{
					Min: cmin, Max: cmax,
					Data: childNode{cmin, cmax, cn},
					Item: false,
				})
			}
		}
	}
	return children
}

// getChildNodeIndex returns the child node rect and index from the row x/y
// coordinates.
func (n *node) getChildNodeIndex(nmin, nmax [2]float64, cx, cy int,
) (cidx int, cmin, cmax [2]float64) {
	cnw := (nmax[0] - nmin[0]) / rows // width of each node
	cnh := (nmax[1] - nmin[1]) / rows // height of each node
	cmin = [2]float64{
		cnw*float64(cx) + nmin[0], // node min x
		cnh*float64(cy) + nmin[1], // node max x
	}
	cmax = [2]float64{
		cmin[0] + cnw, // node min y
		cmin[1] + cnh, // node max y
	}
	cidx = calcNodeIndex(cx, cy)
	return
}
