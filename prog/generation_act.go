package prog

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Junjie-Fan/tfidf"
)

var tfidfLock sync.Mutex

func (target *Target) GenerateACT(rs rand.Source, ncalls int, ct *ChoiceTable, callpus *tfidf.TFIDF, psyzFlags PsyzFlagType) *Prog {
	if (psyzFlags&PsyzRandomW != 0) && (psyzFlags&PsyzTFIDF != 0) {
		return target.GenerateRandomWTFIDF(rs, ncalls, ct, callpus)
	}
	if psyzFlags&PsyzRandomW != 0 {
		return target.GenerateRandomW(rs, ncalls, ct)
	}
	if psyzFlags&PsyzTFIDF != 0 {
		return target.GenerateTFIDF(rs, ncalls, ct, callpus)
	}

	return target.Generate(rs, ncalls, ct)
}

// ===================================================================================================
// Generate SCS based only on TFIDF + ACT
func (target *Target) GenerateTFIDF(rs rand.Source, ncalls int, ct *ChoiceTable, callpus *tfidf.TFIDF) *Prog {
	p := &Prog{
		Target: target,
	}
	r := newRand(target, rs)
	s := newState(target, ct, nil)
	for len(p.Calls) < ncalls {
		calls := r.generateCallwithTFIDF(s, p, len(p.Calls), callpus)
		for _, c := range calls {
			s.analyze(c)
			p.Calls = append(p.Calls, c)
		}
	}
	// For the last generated call we could get additional calls that create
	// resources and overflow ncalls. Remove some of these calls.
	// The resources in the last call will be replaced with the default values,
	// which is exactly what we want.
	for len(p.Calls) > ncalls {
		p.RemoveCall(ncalls - 1)
	}
	p.sanitizeFix()
	p.debugValidate()
	return p
}

func (r *randGen) generateCallwithTFIDF(s *state, p *Prog, insertionPoint int, callpus *tfidf.TFIDF) []*Call {
	biasCall := -1
	if insertionPoint > 0 {
		// Choosing the base call is based on the insertion point of the new calls sequence.
		insertionCall := p.Calls[r.Intn(insertionPoint)].Meta
		if !insertionCall.Attrs.NoGenerate {
			// We must be careful not to bias towards a non-generatable call.
			biasCall = insertionCall.ID
		}
	}
	idx := s.ct.choosewithTFIDF(r.Rand, biasCall, callpus)
	meta := r.target.Syscalls[idx]
	return r.generateParticularCall(s, meta)
}

func (ct *ChoiceTable) choosewithTFIDF(r *rand.Rand, bias int, callpus *tfidf.TFIDF) int {
	if r.Intn(100) < 5 {
		// Let's make 5% decisions totally at random.
		return ct.calls[r.Intn(len(ct.calls))].ID
	}
	if bias < 0 {
		for {
			//bias = ct.calls[r.Intn(len(ct.calls))].ID
			if callpus.N == 0 {
				bias = ct.calls[r.Intn(len(ct.calls))].ID
			} else {
				tfidfLock.Lock()
				p := SortMapByValue(callpus.Allterms)
				tfidfLock.Unlock()
				zero, nozero := p.ZeroDevid()
				random := r.Float64()
				if len(zero) != 0 && random <= 0.618 {
					index := r.Intn(len(zero))
					bias = zero[index].Key
				} else {
					chooserun := make([]float32, len(nozero))
					for i, v := range nozero {
						chooserun[i] = float32(1) / float32(v.Value)
					}
					for i := 1; i < len(chooserun); i++ {
						chooserun[i] += chooserun[i-1]
					}
					x := float32(r.Intn(int(chooserun[len(chooserun)-1])))
					index := sort.Search(len(chooserun), func(i int) bool { return chooserun[i] >= x })
					bias = nozero[index].Key
				}
			}
			if ct.Generatable(bias) {
				break
			}
		}
	}

	if !ct.Generatable(bias) {
		fmt.Printf("bias to disabled or non-generatable syscall %v\n", ct.target.Syscalls[bias].Name)
		panic("disabled or non-generatable syscall")
	}
	run := ct.runs[bias]
	runSum := int(run[len(run)-1])
	x := int32(r.Intn(runSum) + 1)
	res := sort.Search(len(run), func(i int) bool {
		return run[i] >= x
	})
	if !ct.Generatable(res) {
		panic("selected disabled or non-generatable syscall")
	}
	return res
}

// ===================================================================================================
// Generate SCS based only on RandomWalk + ACT
func (target *Target) GenerateRandomW(rs rand.Source, ncalls int, ct *ChoiceTable) *Prog {
	p := &Prog{
		Target: target,
	}
	r := newRand(target, rs)
	s := newState(target, ct, nil)
	ngragh := r.GenGraph(ncalls/2+1, ct)
	calls := GenFromGraph(ngragh)
	//target.InitTargetSeqPoll(calls, *ngragh)
	//for _,call:=range calls {
	call := calls[0] //先实验一波
	for _, item := range call {
		idx := ngragh.IDmap[item]
		meta := s.target.Syscalls[idx]
		gencalls := r.generateParticularCall(s, meta)
		for _, c := range gencalls {
			s.analyze(c)
			p.Calls = append(p.Calls, c)
		}
	}
	for len(p.Calls) > ncalls {
		p.RemoveCall(ncalls - 1)
	}
	p.sanitizeFix()
	p.debugValidate()
	return p
}

func (r *randGen) GenGraph(ncalls int, ct *ChoiceTable) *NGraph {
	globalVisit := make([]int, 0)
	ngraph := &NGraph{
		IDmap: make(map[int]int),
		Graph: make([][]int, ncalls),
	}
	choose, _, _ := r.ChooseOne(globalVisit, ct, true) //选择第一个调用
	globalVisit = append(globalVisit, choose)
	ngraph.Graph[0] = make([]int, ncalls)
	ngraph.IDmap[0] = choose
	i := 1
	var dir int
	for len(globalVisit) < ncalls {
		before := 0
		choose, dir, before = r.ChooseOne(globalVisit, ct, false)
		globalVisit = append(globalVisit, choose)
		ngraph.IDmap[i] = choose
		if dir == 0 { //向后扩展了一个调用，开辟该调用的空间
			ngraph.Graph[i] = make([]int, ncalls)
		}
		AppendGraph(ngraph, before, i, dir, ncalls)
		i++
	}
	return ngraph
}
func (r *randGen) ChooseOne(globalVisit []int, ct *ChoiceTable, isFirst bool) (id int, dir int, bias int) {
	Prope, Fre := r.BuildTwoGramTable()

	if isFirst { //选首个调用，选取随机值
		//x := rand.Intn(len(ct.runs))  //不能随机选，会遇到None generatable的调用
		x := ct.calls[r.Intn(len(ct.calls))].ID
		return x, 0, -1

	} else { //其他位置调用
		rd := r.Intn(2)                       //随机数选前面还是后面
		biasIndex := r.Intn(len(globalVisit)) //选择bias的下标随机数
		bias := globalVisit[biasIndex]
		//1选前向
		if rd == 1 {
			if key := ct.NgramChooseFront(r.Rand, Fre, globalVisit, bias); (key != -1) && ct.Generatable(key) { //在N-gram表中没有该项
				return key, rd, biasIndex
			} else if key2 := ct.chooseFront(r.Rand, globalVisit, bias); (key2 != -1) && ct.Generatable(key2) {

				return key2, rd, biasIndex
			} else {
				rd = 0
				key3 := ct.choose(r.Rand, bias)
				return key3, rd, biasIndex
			}
		} else { //0选后向
			if key := ct.NgramChoose(r.Rand, Prope, globalVisit, bias); (key != -1) && ct.Generatable(key) {
				return key, rd, biasIndex
			} else {
				key3 := ct.choose(r.Rand, bias)
				return key3, rd, biasIndex
			}
		}
	}
}

func (ct *ChoiceTable) NgramChooseFront(r *rand.Rand, prope map[int]map[int]int32, globalVisit []int, bias int) int { //根据ngram选前一个
	ret := -1
	var run []int32
	var id []int
	for k0, v0 := range prope {
		for k1, v1 := range v0 {
			if k1 == bias && NotInSlice(k0, globalVisit) {
				run = append(run, v1)
				id = append(id, k0)
			}
		}
	}
	if run == nil {
		return ret
	}
	for i := 1; i < len(run); i++ {
		run[i] += run[i-1]
	}

	fmt.Printf("NgramChooseFront: value of run[len(run)-1]: %d\n", run[len(run)-1])
	x := int32(r.Intn(int(run[len(run)-1])) + 1)
	ret = sort.Search(len(run), func(i int) bool {
		return run[i] >= x
	})
	return id[ret]
}

func (ct *ChoiceTable) chooseFront(r *rand.Rand, globalVisit []int, bias int) int { //choiceTable根据bias选前一个
	runs := ct.runs
	var run []int32
	for _, v0 := range runs {
		if v0 == nil { //必须保证run的完整，否则返回的bias 不正确		//
			run = append(run, 0) //
		} //
		if v0 != nil {
			if bias == 0 {
				run = append(run, v0[bias])
			} else {
				run = append(run, v0[bias]-v0[bias-1])
			}
		}
	}
	if run == nil {
		return -1
	}
	for i := 1; i < len(run); i++ {
		run[i] += run[i-1]
	}

	if int(run[len(run)-1]) <= 0 { //这是显然存在的情况
		return -1
	}

	fmt.Printf("chooseFront: value of run[len(run)-1]: %d\n", run[len(run)-1])
	x := int32(r.Intn(int(run[len(run)-1])) + 1)
	res := sort.Search(len(run), func(i int) bool {
		return run[i] >= x
	})
	// if !ct.Generatable(res) {
	// 	panic("selected disabled or non-generatable syscall")
	// }
	for ; res < len(run); res++ { //选一个不在globalVisit的调用
		if NotInSlice(res, globalVisit) {
			return res
		}
	}
	return -1
}

func (ct *ChoiceTable) NgramChoose(r *rand.Rand, prope map[int]map[int]float32, globalVisit []int, bias int) int { //根据ngram选后一个
	if bias < 0 {
		var callslice []int
		for k := range prope {
			callslice = append(callslice, k)
		}
		biasID := r.Intn(len(callslice))
		bias = callslice[biasID]
	}
	//run := prope[bias]
	run := make(map[int]float32)
	for k, v := range prope[bias] {
		run[k] = v
	}
	if len(run) == 0 || run == nil {
		return -1
	}
	for i := 1; i < len(run); i++ {
		run[i] += run[i-1]
	}
	x := r.Float32()
	res := sort.Search(len(run), func(i int) bool {
		return run[i] >= x
	})
	for ; res < len(run); res++ {
		if NotInSlice(res, globalVisit) {
			return res
		}
	}
	return -1
}

// ===================================================================================================
// Generate SCS based only on RandomWalk + TFIDF + ACT
func (target *Target) GenerateRandomWTFIDF(rs rand.Source, ncalls int, ct *ChoiceTable, callpus *tfidf.TFIDF) *Prog {
	p := &Prog{
		Target: target,
	}
	r := newRand(target, rs)
	s := newState(target, ct, nil)
	ngragh := r.GenGraphTFIDF(rs, ncalls/2+1, ct, callpus)
	calls := GenFromGraph(ngragh)
	//target.InitTargetSeqPoll(calls, *ngragh)	//没有后续处理，毫无意义的函数
	//for _,call:=range calls {
	call := calls[0] //先实验一波
	for _, item := range call {
		idx := ngragh.IDmap[item]
		meta := s.target.Syscalls[idx]
		gencalls := r.generateParticularCall(s, meta)
		for _, c := range gencalls {
			s.analyze(c)
			p.Calls = append(p.Calls, c)
		}
	}
	for len(p.Calls) > ncalls {
		p.RemoveCall(ncalls - 1)
	}
	p.sanitizeFix()
	p.debugValidate()
	return p
}

func (r *randGen) GenGraphTFIDF(rs rand.Source, ncalls int, ct *ChoiceTable, callpus *tfidf.TFIDF) *NGraph {
	globalVisit := make([]int, 0)
	ngraph := &NGraph{
		IDmap: make(map[int]int),
		Graph: make([][]int, ncalls),
	}
	choose, _, _ := r.ChooseOneTFIDF(rs, globalVisit, ct, true, callpus) //选择第一个调用
	globalVisit = append(globalVisit, choose)
	ngraph.Graph[0] = make([]int, ncalls)
	ngraph.IDmap[0] = choose
	i := 1
	var dir int
	for len(globalVisit) < ncalls {
		before := 0
		choose, dir, before = r.ChooseOneTFIDF(rs, globalVisit, ct, false, callpus)
		globalVisit = append(globalVisit, choose)
		ngraph.IDmap[i] = choose
		if dir == 0 { //向后扩展了一个调用，开辟该调用的空间
			ngraph.Graph[i] = make([]int, ncalls)
		}
		AppendGraph(ngraph, before, i, dir, ncalls)
		i++
	}
	return ngraph
}

func (r *randGen) ChooseOneTFIDF(rs rand.Source, globalVisit []int, ct *ChoiceTable, isFirst bool, callpus *tfidf.TFIDF) (id int, dir int, bias int) {
	Prope, Fre := r.BuildTwoGramTable()

	if isFirst { //选首个调用，选取随机值
		//x := rand.Intn(len(ct.runs)) //不能随机选，会遇到None generatable的调用
		x := ct.calls[r.Intn(len(ct.calls))].ID
		return x, 0, -1

	} else { //其他位置调用
		rd := r.Intn(2) //随机数选前面还是后面TODO:maybe need a change!!!!!//20230306更改
		var s []string
		for i := 0; i < len(globalVisit); i++ {
			s = append(s, strconv.Itoa(globalVisit[i]))
		}
		resString := strings.Join(s, " ")

		tfidfLock.Lock()
		tfidfval := callpus.Cal(resString)
		tfidfLock.Unlock()

		biasIndex := chooseIndexFromMap(r.Rand, tfidfval, globalVisit)
		bias := globalVisit[biasIndex]
		//1选前向
		if rd == 1 {
			if key := ct.NgramChooseFront(r.Rand, Fre, globalVisit, bias); (key != -1) && ct.Generatable(key) { //在N-gram表中没有该项
				return key, rd, biasIndex
			} else if key2 := ct.chooseFront(r.Rand, globalVisit, bias); (key2 != -1) && ct.Generatable(key2) {
				return key2, rd, biasIndex
			} else {
				rd = 0
				key3 := ct.choose(r.Rand, bias)
				return key3, rd, biasIndex
			}
		} else { //0选后向
			if key := ct.NgramChoose(r.Rand, Prope, globalVisit, bias); (key != -1) && ct.Generatable(key) {
				return key, rd, biasIndex
			} else {
				key3 := ct.choose(r.Rand, bias)
				return key3, rd, biasIndex
			}
		}
	}
}

func chooseIndexFromMap(r *rand.Rand, m map[string]float64, g []int) int {
	if len(g) == 1 {
		return 0
	}
	run := make([]float64, len(m))
	for i := 0; i < len(m); i++ {
		run[i] = m[strconv.Itoa(g[i])]
	}
	for i := 1; i < len(m); i++ {
		run[i] += run[i-1]
	}

	x := r.Float64()
	res := sort.Search(len(run), func(i int) bool {
		return run[i] >= x
	})
	if res >= len(g) {
		//first time run chooseIndexFromMap, corpus could be nil, then g[res] could overflow here
		// return a random result

		res = int(x * float64(res))
		//fmt.Printf("return random result: %d\n", res)
		//res = len(g) - 1
	}
	return res
}

// ===================================================================================================
// Base Function Retated to GenGraph

type Pair struct { //这个结构体用于根据map的value值排序map
	Key   int
	Value int
}

type PairList []Pair

func SortMapByValue(m map[int]int) PairList {
	p := make(PairList, len(m))
	i := 0
	for k, v := range m {
		p[i] = Pair{k, v}
		i++
	}
	sort.Sort(p)
	return p
}

func (p PairList) ZeroDevid() (zero PairList, nozero PairList) {
	zero = make(PairList, 0)
	nozero = make(PairList, 0)
	for _, v := range p {
		if v.Value == 0 {
			zero = append(zero, v)
		} else {
			nozero = append(nozero, v)
		}
	}
	return zero, nozero
}

func (p PairList) Less(i, j int) bool {
	return p[i].Value < p[j].Value
}

func (p PairList) Len() int {
	return len(p)
}

func (p PairList) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (r *randGen) BuildTwoGramTable() (map[int]map[int]float32, map[int]map[int]int32) {
	if Twogram == nil {
		return nil, nil
	}
	return Twogram.Prope, Twogram.Fre
}

func GenFromGraph(ngraph *NGraph) [][]int {
	return TopoSortSimple(ngraph.Graph)
}

func AppendGraph(ngraph *NGraph, before int, choose int, dir int, ncalls int) { //扩展选择的子图
	if dir == 1 {
		ngraph.Graph[choose] = make([]int, ncalls) //如果是选前向，需要开辟数组
		ngraph.Graph[choose][before] = 1           //choose后是before
	} else {
		ngraph.Graph[before][choose] = 1 //before后是choose
	}
}

func NotInSlice(call int, globalVisit []int) bool {
	ret := true
	for _, v := range globalVisit {
		if v == call {
			ret = false
			break
		}
	}
	return ret
}
