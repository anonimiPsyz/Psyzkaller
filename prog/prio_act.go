package prog

import (
	"encoding/json"
	"fmt"
	"math"
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
	PsyzDongTingSyzk
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
	fmt.Printf("Build Choice Table... %s\n", psyzModeStr)

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
	} else if psyzFlags&PsyzDongTingSyzk != 0 {
		initNRSuccessorPropeSyzk(generatableCalls, succJsonData)
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
	if (psyzFlags&PsyzNgram != 0) || (psyzFlags&PsyzDongTing != 0) || (psyzFlags&PsyzDongTingSyzk != 0) {
		//fmt.Printf("Build Choice Table use CalculatePrioritiesACT\n")
		prios = target.CalculatePrioritiesACT(corpus, psyzFlags)
	} else {
		//fmt.Printf("Build Choice Table use CalculatePriorities\n")
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

	//target.WriteCTToJson(run, "choiceTable.json")
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
	if (psyzFlags & PsyzDongTingSyzk) != 0 {
		flagStr += "DongTingSyzk "
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

	//addDTTotal := 0
	//addZeroDTTotal := 0

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
					//addDTTotal += 1
					//if prop == 0 {
					//	addZeroDTTotal += 1
					//}
				}
			}
		}
	}
	//fmt.Printf("DongTing Mode: addDTTotal:%d\n", addDTTotal)
	//fmt.Printf("DongTing Mode: addZeroDTTotal:%d\n", addZeroDTTotal)
	//os.Exit(-1)
	return ret_obj
}

// translate linux syscall ID to syzkaller's syscall ID, and disable Ungeneratable sysCalls.
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

// compare with initNRSuccessorPrope(), this function only need to disable Ungeneratable sysCalls.
func initNRSuccessorPropeSyzk(generatableCalls []*Syscall, succJsonData []uint8) {
	generatableIDs := make(map[int]bool)
	for _, syscallTmp := range generatableCalls {
		generatableIDs[syscallTmp.ID] = true
	}

	jsonData := succJsonData

	infor_obj := make(map[int]map[int]float32)
	err := json.Unmarshal(jsonData, &infor_obj)
	if err != nil {
		fmt.Println("JSON 解码失败:", err)
		os.Exit(-1)
	}
	if len(infor_obj) == 0 {
		os.Exit(-1)
	}

	ret_obj := make(map[int]map[int]float32)
	for headID, sucMap := range infor_obj {
		if _, ok := generatableIDs[headID]; !ok {
			continue
		}
		for sucID, prop := range sucMap {
			if _, ok := generatableIDs[sucID]; !ok {
				continue
			}
			if _, ok := ret_obj[headID]; !ok {
				ret_obj[headID] = make(map[int]float32)
			}
			ret_obj[headID][sucID] = prop

		}

	}
	NRSuccessorPrope = ret_obj
}

// ============================================================================================
// CalculatePrioritiesACT, N-gram and DongTing related code
func (target *Target) CalculatePrioritiesACT(corpus []*Prog, psyzFlags PsyzFlagType) [][]int32 {
	//fmt.Printf("CalculatePrioritiesACT...\n")
	var pd int32 = 1
	static := target.calcStaticPriorities()
	if len(corpus) != 0 {
		// Let's just sum the static and dynamic distributions.
		//static := target.calcDynamicACT(corpus, static, psyzFlags) 
		target.calcDynamicACT(corpus, static, psyzFlags) 
		dynamic := target.calcDynamicPrio(corpus)
		for i, prios := range dynamic {
			dst := static[i]
			for j, p := range prios {
				//dst[j] += p
				dst[j] += (p * pd)
			}

		}
	}
	return static
}

func (target *Target) calcDynamicACT(corpus []*Prog, static [][]int32, psyzFlags PsyzFlagType) {
	//fmt.Printf("calcDynamicACT...\n")
	//normalizeFactor := float64(10 * int32(len(target.Syscalls))) // comes from function normalizePrios()
	var ps int32 = 1
	var pdn int32 = 1
	var pdtn int32 = 1
	total := 10 * float64(len(target.Syscalls))

	ngramDynamic := make([][]int32, len(target.Syscalls))
	for i := range ngramDynamic {
		ngramDynamic[i] = make([]int32, len(target.Syscalls))
	}

	if (psyzFlags&PsyzNgram) != 0 && len(corpus) > 1000 {
		PropeLock.Lock()
		Twogram = MakeTwoGram()
		for _, p := range corpus {
			if len(p.Calls) > 1 {
				Twogram.NoGenPathCalFre(p)
			}
		}
		Twogram.CalculateProbalility()

		for i, v0 := range Twogram.Prope {
			for j, v1 := range v0 {
				//ngramDynamic[i][j] = int32(normalizeFactor * v1)
				ngramDynamic[i][j] = int32(total * 2.0 * math.Sqrt(float64(v1)))
			}
		}
		PropeLock.Unlock()
		normalizePriosBigNum(ngramDynamic)
	}

	dtNgramDynamic := make([][]int32, len(target.Syscalls))
	for i := range dtNgramDynamic {
		dtNgramDynamic[i] = make([]int32, len(target.Syscalls))
	}

	if ((psyzFlags & PsyzDongTing) != 0) || ((psyzFlags & PsyzDongTingSyzk) != 0) {
		for i, v0 := range NRSuccessorPrope {
			for j, v1 := range v0 {
				//dtNgramDynamic[i][j] += int32(normalizeFactor * v1)
				//dtNgramDynamic[i][j] += int32( 2.0 * math.Sqrt(float64(v1)))
				dtNgramDynamic[i][j] = int32(total * 2.0 * math.Sqrt(float64(v1)))
			}
		}
		normalizePriosBigNum(dtNgramDynamic)

		PropeLock.Lock()
		Twogram = MakeTwoGram()
		for i, v0 := range dtNgramDynamic {
			itotal := 0
			for _, v1 := range v0 {
				//Twogram.Prope[i][j] += float32(v1)
				itotal += int(v1)
			}
			for j, v1 := range v0 {
				if itotal != 0 {
					if Twogram.Prope[i] == nil {
						Twogram.Prope[i] = make(map[int]float32)
					}
					Twogram.Prope[i][j] += float32(v1) / float32(itotal)
				}
			}
		}

		for i, v0 := range Twogram.Prope {
			ftotal := float32(0.0)
			for _, v1 := range v0 {
				ftotal += v1
			}
			for j, v1 := range v0 {
				if ftotal != 0 {
					if Twogram.Prope[i] == nil {
						Twogram.Prope[i] = make(map[int]float32)
					}
					Twogram.Prope[i][j] = v1 / ftotal
				}
			}
		}
		PropeLock.Unlock()
	}

	for i := range static {
		for j := range static[i] {
			//static[i][j] = static[i][j] + ngramDynamic[i][j] + dtNgramDynamic[i][j]
			static[i][j] = static[i][j]*ps + ngramDynamic[i][j]*pdn + dtNgramDynamic[i][j]*pdtn
		}
	}
	//target.printSomePairInform(static, dtNgramDynamic)
	//analy_static_dt_ngram_result(ret, static, ngramDynamic, dtNgramDynamic)

	//return ret
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

// normalizePrio distributes |N| * 10 points proportional to the values in the matrix.
// 避免p*tatal过大导致整数溢出
func normalizePriosBigNum(prios [][]int32) {
	total := 10 * int32(len(prios))
	for _, prio := range prios {
		sum := int32(0)
		for _, p := range prio {
			sum += p
		}
		if sum == 0 {
			continue
		}
		for i, p := range prio {
			prio[i] = int32(float64(p) * float64(total) / float64(sum))
		}
	}
}

// ============================================================================================
func (target *Target) printSomePairInform(static [][]int32, dt [][]int32) {
	linux2Syzcall := make([][]int, 600) //kernel syscall number should be about 440+, set 600 to insure enough

	for _, s := range target.Syscalls {
		linux2Syzcall[s.NR] = append(linux2Syzcall[s.NR], s.ID)
	}

	IDtoNR := make(map[int]int)
	for k, v := range NRtoIDlist {
		for _, v1 := range v {
			IDtoNR[v1] = int(k)
		}
	}

	tatalPair := uint64(0)
	outstandPair := uint64(0)
	zeroPair := uint64(0) 
	totalNRPair := uint64(0)
	totalZeroNRPair := uint64(0)
	belowAvaPair := uint64(0)
	totalNoneZeroPair := uint64(0)

	NRtoNRUsed := make(map[int]map[int]bool)
	for i, vl := range dt {
		for j, _ := range vl {
			if !NRtoNRUsed[i][j] && dt[i][j] > 0 {
				NR_caller := IDtoNR[i]
				NR_callee := IDtoNR[j]
				callerList := NRtoIDlist[uint64(NR_caller)]
				calleeList := NRtoIDlist[uint64(NR_callee)]
				tatalPair += uint64(len(callerList) * len(calleeList))

				totalPrio := uint64(0)
				noneZeroPair := uint64(0)
				everagePrio := uint64(0)
				totalNRPair += 1

				fmt.Printf("NR_caller: %v NR_callee: %v\n", NR_caller, NR_callee)
				fmt.Printf("callerList: %v \ncalleeList: %v\n", callerList, calleeList)
				fmt.Printf("dt[%d][%d] = %d \n", i, j, dt[i][j])

				for _, c1 := range callerList {
					for _, c2 := range calleeList {
						//fmt.Printf("dt[%d][%d]=%d  ", c1, c2, dt[c1][c2])
						if NRtoNRUsed[c1] == nil {
							NRtoNRUsed[c1] = make(map[int]bool)
						}
						NRtoNRUsed[c1][c2] = true
					}
				}

				for _, c1 := range callerList {
					//fmt.Printf("static[%d] ---    ", c1)
					for _, c2 := range calleeList {
						if static[c1][c2] > 0 {
							//fmt.Printf("[%d]=%d|%d  ", c2, static[c1][c2], dt[c1][c2])
							totalPrio += uint64(static[c1][c2])
							noneZeroPair += 1
							totalNoneZeroPair += 1
							if static[c1][c2] <= 10 {
								belowAvaPair += 1
							}
						}
					}
					//fmt.Printf("\n")
				}

				if noneZeroPair == 0 {
					outstandPair += uint64(len(callerList) * len(calleeList))
					zeroPair += uint64(len(callerList) * len(calleeList))
					totalZeroNRPair += 1
				} else {
					everagePrio = totalPrio / noneZeroPair
					for _, c1 := range callerList {
						for _, c2 := range calleeList {
							if static[c1][c2] > 0 {
								if uint64(static[c1][c2]) >= (2*everagePrio) || uint64(static[c1][c2]) < (everagePrio/2) {
									outstandPair += 1
								} else {
									outstandPair += 1
									zeroPair += 1
								}
							}
						}
					}
				}

			}
		}
	}

	dt_count := uint64(0)
	for i, vl := range dt {
		for j, _ := range vl {
			if dt[i][j] > 0 {
				dt_count += 1
			}
		}
	}

	fmt.Printf("tatalPair: %d\n", tatalPair)
	fmt.Printf("outstandPair: %d\n", outstandPair)
	fmt.Printf("zeroPair: %d\n", zeroPair)
	fmt.Printf("totalNRPair: %d\n", totalNRPair)
	fmt.Printf("totalZeroNRPair: %d\n", totalZeroNRPair)
	fmt.Printf("dt_count: %d\n", dt_count)
	fmt.Printf("belowAvaPair: %d\n", belowAvaPair)
	fmt.Printf("totalNoneZeroPair: %d\n", totalNoneZeroPair)

}

func analy_static_dt_ngram_result(ret, static, ngramDynamic, dtNgramDynamic [][]int32) {
	for i, v0 := range ret {
		totalStatic := int32(0)
		totalngram := int32(0)
		totaldt := int32(0)
		for j, _ := range v0 {
			ret[i][j] = static[i][j] + ngramDynamic[i][j] + dtNgramDynamic[i][j]
			totalStatic += static[i][j]
			totalngram += ngramDynamic[i][j]
			totaldt += dtNgramDynamic[i][j]
		}
		fmt.Printf("totalStatic: %d, totalngram: %d, totaldt: %d\n", totalStatic, totalngram, totaldt)
	}
}

// ============================================================================================
func (target *Target) WriteCTToJson(runs [][]int32, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") 

	err = encoder.Encode(runs)
	if err != nil {
		return err
	}

	return nil
}

// ============================================================================================
func (target *Target) WriteCTToJson2(ct *ChoiceTable, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") 

	err = encoder.Encode(ct.runs)
	if err != nil {
		return err
	}

	return nil
}
