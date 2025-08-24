package prog

import (
	"math/rand"
	"time"
)

type DAG struct { 
	vertex int           
	list   map[int][]int
}

func (g *DAG) addVertex(source int, dest int) {
	g.list[source] = push(g.list[source], dest)
}

func pop(list []int) (int, []int) {
	k := len(list)
	if k > 0 {
		a := list[k-1]
		b := list[0 : k-1]
		return a, b
	} else {
		return -1, list
	}
}

func push(list []int, value int) []int {
	result := append(list, value)
	return result
}

func NewGraph(v int) *DAG {
	g := new(DAG)
	g.vertex = v
	g.list = map[int][]int{}
	i := 0
	for i < v {
		g.list[i] = make([]int, 0)
		i++
	}
	return g
}

func InitfromMatrix(matrix [][]int) *DAG { 
	dag := NewGraph(len(matrix))
	for i, v0 := range matrix {
		for j, v1 := range v0 {
			if v1 == 1 {
				dag.addVertex(i, j)
			}
		}
	}
	return dag
}

func (dag *DAG) MinusInDegree(i int, inDegree map[int]int) { 
	for _, k := range dag.list[i] {
		inDegree[k]--
	}
}

func (dag *DAG) ADDInDegree(i int, inDegree map[int]int) { 
	for _, k := range dag.list[i] {
		inDegree[k]++
	}
}

func TopoSort(matrix [][]int) [][]int {
	dag := InitfromMatrix(matrix)
	var inDegree = make(map[int]int)
	//var queue []int
	for i := 0; i < dag.vertex; i++ {
		inDegree[i] = 0
	}
	for i := 1; i <= dag.vertex; i++ {
		for _, m := range dag.list[i] {
			inDegree[m]++
		}
	}
	path := make([]int, 0)
	visited := make([]bool, dag.vertex)
	paths := make([][]int, 0)
	//===========
	var curse func(dag *DAG, inDegree map[int]int, path []int, visited []bool, node int) 
	curse = func(dag *DAG, inDegree map[int]int, path []int, visited []bool, start int) {
		if len(path) == dag.vertex {
			temp := make([]int, len(path))
			copy(temp, path)
			paths = append(paths, temp)
		}
		for i := 0; i < dag.vertex; i++ {
			if inDegree[i] == 0 && !visited[i] {
				path = push(path, i)
				dag.MinusInDegree(i, inDegree)
				visited[i] = true
				curse(dag, inDegree, path, visited, i)
				_, path = pop(path)
				dag.ADDInDegree(i, inDegree)
				visited[i] = false
			}

		}
	}
	//==========
	for i := 0; i < dag.vertex; i++ {
		if inDegree[i] == 0 {
			path = push(path, i)
			dag.MinusInDegree(i, inDegree)
			visited[i] = true
			curse(dag, inDegree, path, visited, i)
			_, path = pop(path)
			dag.ADDInDegree(i, inDegree)
			visited[i] = false
		}

	}
	return paths
}

func findMinimumIndgreeNode(inDegree map[int]int, visited []bool, r *rand.Rand) int {
	minimize := 20000000
	ret := -1
	retL := make([]int, 0)

	for i := 0; i < len(inDegree); i++ {
		if inDegree[i] < minimize && !visited[i] {
			retL = []int{i}
			minimize = inDegree[ret]
		}
		if inDegree[i] == minimize && !visited[i] {
			retL = append(retL, i)
		}
	}

	if len(retL) > 1 {
		ret = retL[r.Intn(len(retL))]
	}
	if len(retL) == 1 {
		ret = retL[ret]
	}
	return ret
}

func TopoSortSimple(matrix [][]int) [][]int {
	rs := rand.NewSource(time.Now().UnixNano())
	r := rand.New(rs)

	dag := InitfromMatrix(matrix)
	var inDegree = make(map[int]int)

	for i := 0; i < dag.vertex; i++ {
		inDegree[i] = 0
	}
	for i := 0; i < dag.vertex; i++ {
		for _, m := range dag.list[i] {
			inDegree[m]++
		}
	}

	path := make([]int, 0)
	visited := make([]bool, dag.vertex)
	paths := make([][]int, 0)
	//===========

	for i := 0; i < dag.vertex; i++ {
		nextNode := findMinimumIndgreeNode(inDegree, visited, r)
		visited[nextNode] = true
		path = push(path, nextNode)
		dag.MinusInDegree(nextNode, inDegree)
	}

	paths = append(paths, path)
	return paths
}
