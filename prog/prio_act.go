package prog

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Junjie-Fan/tfidf"
)

// ============================================================================================
// Psyzkaller related variables and types
var NRtoIDlist = make(map[uint64][]int)
var NRSuccessorPrope map[int]map[int]float32
var Twogram *TwoGramTable

type PsyzFlagType int

const (
	PsyzNgram PsyzFlagType = 1 << iota
	PsyzTFIDF
	PsyzRandomW
	PsyzDongTing
	PsyzMix
)

// ============================================================================================
// Psyzkaller's Main function
func (target *Target) BuildChoiceTablePsyz(corpus []*Prog, enabled map[*Syscall]bool, succJsonData []uint8, psyzFlags PsyzFlagType) (*ChoiceTable, *tfidf.TFIDF) {
	// None psyzkaller mode enebled, call syzkaller's BuildChoiceTable() and return
	if psyzFlags == 0 {
		return target.BuildChoiceTable(corpus, enabled), nil
	}

	// Print psyzkaller enabled mode
	var psyzModeStr string = getPsyzFlagStr(psyzFlags)
	fmt.Printf("Build Choice Table... %s", psyzModeStr)

	//syzkaller ori code
	if enabled == nil {
		enabled = make(map[*Syscall]bool)
		for _, c := range target.Syscalls {
			enabled[c] = true
		}
	}
	noGenerateCalls := make(map[int]bool)
	enabledCalls := make(map[*Syscall]bool)
	for call := range enabled {
		if call.Attrs.NoGenerate {
			noGenerateCalls[call.ID] = true
		} else if !call.Attrs.Disabled {
			enabledCalls[call] = true
		}
	}
	var generatableCalls []*Syscall
	for c := range enabledCalls {
		generatableCalls = append(generatableCalls, c)
	}
	if len(generatableCalls) == 0 {
		panic("no syscalls enabled and generatable")
	}
	sort.Slice(generatableCalls, func(i, j int) bool {
		return generatableCalls[i].ID < generatableCalls[j].ID
	})

	for _, p := range corpus {
		for _, call := range p.Calls {
			if !enabledCalls[call.Meta] && !noGenerateCalls[call.Meta.ID] {
				fmt.Printf("corpus contains disabled syscall %v\n", call.Meta.Name)
				for call := range enabled {
					fmt.Printf("%s: enabled\n", call.Name)
				}
				panic("disabled syscall")
			}
		}
	}

	// DongTing Mode related code
	if psyzFlags&PsyzDongTing != 0 {
		initNRSuccessorPrope(generatableCalls, succJsonData)
	} else {
		NRSuccessorPrope = nil
	}

	//psyzkaller TF-IDF Mode related code
	callpus := tfidf.New()
	for _, v := range generatableCalls {
		callpus.InitTerms(v.ID)
	}
	if psyzFlags&PsyzTFIDF != 0 {
		target.CalculatePrioritiesTFIDF(corpus, callpus)
		//fmt.Printf("CalculatePrioritiesTFIDF: callpus.N:%d\n", callpus.N)
	}

	// psyzkaller CalculatePrioritiesACT, N-gram and DongTing related code
	var prios [][]int32
	if (psyzFlags&PsyzNgram != 0) || (psyzFlags&PsyzDongTing != 0) {
		prios = target.CalculatePrioritiesACT(corpus, psyzFlags)
	} else {
		prios = target.CalculatePriorities(corpus)
	}

	//syzkaller's ori code
	run := make([][]int32, len(target.Syscalls))
	// ChoiceTable.runs[][] contains cumulated sum of weighted priority numbers.
	// This helps in quick binary search with biases when generating programs.
	// This only applies for system calls that are enabled for the target.
	for i := range run {
		if !enabledCalls[target.Syscalls[i]] {
			continue
		}
		run[i] = make([]int32, len(target.Syscalls))
		var sum int32
		for j := range run[i] {
			if enabledCalls[target.Syscalls[j]] {
				sum += prios[i][j]
			}
			run[i][j] = sum
		}
	}

	return &ChoiceTable{target, run, generatableCalls}, callpus
}

// ============================================================================================
// function for Pre-processing
func getPsyzFlagStr(psyzFlags PsyzFlagType) string {
	var flagStr string
	flagStr = ""
	if (psyzFlags & PsyzNgram) != 0 {
		flagStr += "Ngram "
	}
	if (psyzFlags & PsyzTFIDF) != 0 {
		flagStr += "TFIDF "
	}
	if (psyzFlags & PsyzRandomW) != 0 {
		flagStr += "RandomW "
	}
	if (psyzFlags & PsyzDongTing) != 0 {
		flagStr += "DongTing "
	}
	if (psyzFlags & PsyzMix) != 0 {
		flagStr += "MixGenerateOpti "
	}

	if flagStr == "" {
		flagStr = "Psyzkaller Disabled.\n"
	} else {
		flagStr = "Psyzkaller Enabled Modes: " + flagStr + ".\n"
	}
	return flagStr
}

// ============================================================================================
// function for DongTing Mode
func readAndCalcuNRSuccessorPrope(succJsonData []uint8) map[int]map[int]float32 {
	jsonData := succJsonData

	infor_obj := make(map[int]map[int]float32)
	err := json.Unmarshal(jsonData, &infor_obj)
	if err != nil {
		fmt.Println("JSON 解码失败:", err)
		return nil
	}
	if len(infor_obj) == 0 {
		os.Exit(-1)
	}

	ret_obj := make(map[int]map[int]float32)
	for head, sucMap := range infor_obj {
		headList := NRtoIDlist[uint64(head)]
		for sucNR, prop := range sucMap {
			sucList := NRtoIDlist[uint64(sucNR)]
			for _, headID := range headList {
				for _, sucID := range sucList {
					if _, ok := ret_obj[headID]; !ok {
						ret_obj[headID] = make(map[int]float32)
					}
					ret_obj[headID][sucID] = prop
				}
			}
		}
	}
	return ret_obj
}

func initNRSuccessorPrope(generatableCalls []*Syscall, succJsonData []uint8) {
	for _, syscallTmp := range generatableCalls {
		tNR := syscallTmp.NR
		tID := syscallTmp.ID
		if NRtoIDlist[tNR] == nil {
			NRtoIDlist[tNR] = []int{tID}
		} else {
			NRtoIDlist[tNR] = append(NRtoIDlist[tNR], tID)
		}
	}
	NRSuccessorPrope = readAndCalcuNRSuccessorPrope(succJsonData)
}

// ============================================================================================
// CalculatePrioritiesACT, N-gram and DongTing related code
func (target *Target) CalculatePrioritiesACT(corpus []*Prog, psyzFlags PsyzFlagType) [][]int32 {
	static := target.calcStaticPriorities()
	if len(corpus) != 0 {
		// Let's just sum the static and dynamic distributions.
		if len(corpus) < 1000 {
			dynamic := target.calcDynamicPrio(corpus)
			for i, prios := range dynamic {
				dst := static[i]
				for j, p := range prios {
					dst[j] += p
				}
			}
		} else {
			static := target.calcDynamicACT(corpus, static, psyzFlags) //增强矩阵
			dynamic := target.calcDynamicPrio(corpus)
			for i, prios := range dynamic {
				dst := static[i]
				for j, p := range prios {
					dst[j] += p
				}
			}
		}
	}
	return static
}

func (target *Target) calcDynamicACT(corpus []*Prog, static [][]int32, psyzFlags PsyzFlagType) [][]int32 {
	ret := make([][]int32, len(target.Syscalls))
	for i := range ret {
		ret[i] = make([]int32, len(target.Syscalls))
	}
	copy(ret, static)

	if (psyzFlags & PsyzNgram) != 0 {
		Twogram = MakeTwoGram()
		for _, p := range corpus {
			if len(p.Calls) > 1 {
				Twogram.NoGenPathCalFre(p)
			}
		}
		Twogram.CalculateProbalility()

		for i, v0 := range Twogram.Prope {
			var sum float32
			sum = 0
			for j, _ := range v0 {
				sum += float32(ret[i][j])
			}
			for j, v1 := range v0 {
				//fmt.Println("before", ret[i][j])
				ret[i][j] += int32(sum * v1)
				//fmt.Println("after", ret[i][j])
			}
		}
	}
	normalizePrios(ret)

	ret2 := make([][]int32, len(target.Syscalls))
	for i := range ret2 {
		ret2[i] = make([]int32, len(target.Syscalls))
	}
	copy(ret2, ret)

	if (psyzFlags & PsyzDongTing) != 0 {
		for i, v0 := range NRSuccessorPrope {
			var sum float32
			sum = 0
			for j, _ := range v0 {
				sum += float32(ret2[i][j])
			}
			for j, v1 := range v0 {
				ret2[i][j] += int32(sum * v1)
			}
		}
	}
	normalizePrios(ret2)

	return ret2
}

func (twogram *TwoGramTable) CalculateProbalility() {
	Prope := twogram.Prope
	value := twogram.Fre
	for i, v0 := range value {
		var fenmu float32
		fenmu = 0
		for _, v2 := range v0 {
			fenmu += float32(v2)
		}
		for j, v1 := range v0 {
			if Prope[i] == nil {
				Prope[i] = make(map[int]float32)
			}
			v1f := float32(v1)
			Prope[i][j] = v1f / fenmu
		}
	}
}

// ============================================================================================
// Basic function for Ngram Mode

type NGraph struct {
	Graph [][]int
	IDmap map[int]int
}

type TwoGramTable struct {
	NGraph
	Paths [][]int
	Fre   map[int]map[int]int32
	Prope map[int]map[int]float32
}

func MakeNgraph() *NGraph {
	NewGraph := &NGraph{
		Graph: make([][]int, 0),
		IDmap: make(map[int]int),
	}
	return NewGraph
}
func MakeTwoGram() *TwoGramTable {
	return &TwoGramTable{
		Fre:    make(map[int]map[int]int32),
		Prope:  make(map[int]map[int]float32),
		Paths:  make([][]int, 0),
		NGraph: *MakeNgraph(),
	}
}

func (twogram *TwoGramTable) NoGenPathCalFre(p *Prog) {
	Myfre := twogram.Fre
	path := make([]int, 0)
	for _, v := range p.Calls {
		path = append(path, v.Meta.ID)
	}
	for i := 0; i < len(path)-1; i++ {
		if Myfre[p.Calls[i].Meta.ID] == nil {
			Myfre[p.Calls[i].Meta.ID] = make(map[int]int32)
		}
		if Myfre[p.Calls[i].Meta.ID][p.Calls[i+1].Meta.ID] == 0 {
			Myfre[p.Calls[i].Meta.ID][p.Calls[i+1].Meta.ID] = 1
		} else {
			Myfre[p.Calls[i].Meta.ID][p.Calls[i+1].Meta.ID] += 1
		}
	}
}

// ============================================================================================
// function for TFIDF Mode

func (target *Target) CalculatePrioritiesTFIDF(corpus []*Prog, f *tfidf.TFIDF) {
	if len(corpus) != 0 {
		for i := 0; i < len(corpus); i++ {
			var idString []string
			for _, call := range corpus[i].Calls {
				idString = append(idString, strconv.Itoa(call.Meta.ID))
			}
			resString := strings.Join(idString, " ")
			f.AddDocs(resString)
		}
	}
}
