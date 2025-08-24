//读取指定路径下系统调用序列数据库中的文件，获取每个系统调用的直接后继节点概率，将该数据保存如json文件中

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	syscallIDPath       = flag.String("s", "syscallIDs.txt", "file of syscall ID/name pairs.")                
	syscallSequencePath = flag.String("d", "", "path to DongTing database")                                    
	jsonfile_name       = flag.String("o", "successor_Prope.json", "directory to store deserialized programs") 
)


var syscall_name_2_id = make(map[string]int)
var syscall_id_2_name = make(map[int]string)

//var syscall_sequences = make([][]int, 0)

var successor_frequece = make(map[int]map[int]int32)
var successor_Prope = make(map[int]map[int]float32)

var sequence_count = 0
var syscall_count = 0
var failed_count = 0
var current_count = 0

func readSyscallIDs() {
	// 打开文件
	file, err := os.Open(*syscallIDPath)
	if err != nil {
		fmt.Println("Can not open file：", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		idAndname := strings.Split(line, " ")
		id, err := strconv.Atoi(idAndname[0])
		if err != nil {
			fmt.Println("conv failed, not a number:", err)
			return
		}
		syscall_name_2_id[idAndname[1]] = id
		syscall_id_2_name[id] = idAndname[1]

		//fmt.Println(line)
	}


	if err := scanner.Err(); err != nil {
		fmt.Println("Error while read file:", err)
	}
}

func calculateSuccessorFrequency(seq []int) {
	for i := 0; i < len(seq)-1; i++ {
		if _, ok := successor_frequece[seq[i]]; !ok {
			successor_frequece[seq[i]] = make(map[int]int32)
		}
		successor_frequece[seq[i]][seq[i+1]]++
	}

}

func calculateSuccessorProbalility() {
	for i, v0 := range successor_frequece {
		var fenmu float32
		fenmu = 0
		for _, v2 := range v0 {
			fenmu += float32(v2)
		}
		for j, v1 := range v0 {
			if successor_Prope[i] == nil {
				successor_Prope[i] = make(map[int]float32)
			}
			v1f := float32(v1)
			successor_Prope[i][j] = v1f / fenmu
		}
	}
}

func readOneSequence(path string) {
	current_count++
	// 打开文件
	file, err := os.Open(path)
	if err != nil {
		fmt.Println("Can not open file：", err)
		return
	}
	defer file.Close()

	count := 0

	scanner := bufio.NewScanner(file)


	for scanner.Scan() {
		line := scanner.Text()
		syscallSequence := strings.Split(line, "|")
		count = len(syscallSequence)
		idSequence := make([]int, 0)
		for _, names := range syscallSequence {
			id := syscall_name_2_id[names]
			idSequence = append(idSequence, id)
		}
		//fmt.Println("length:", len(idSequence))
		//fmt.Println(idSequence)
		calculateSuccessorFrequency(idSequence)
		//syscall_sequences = append(syscall_sequences, idSequence)
		syscall_count += len(idSequence)
	}
	fmt.Printf("current_count: %d. File %s has been read, length: %d\n", current_count, path, count)
	if count == 0 {
		failed_count++
	}
}


func processSyscallSequences() {
	err := filepath.Walk(*syscallSequencePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			//fmt.Println(path)
			readOneSequence(path)
			sequence_count++
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error while traversing folder:", err)
	}
}


func saveToJson() {
	jsonData, err := json.MarshalIndent(successor_Prope, "", "    ")
	if err != nil {
		fmt.Println("JSON encoding error:", err)
		return
	}

	file, err := os.Create(*jsonfile_name)
	if err != nil {
		fmt.Println("Creating file failed:", err)
		return
	}
	defer file.Close()

	_, err = file.Write(jsonData)
	if err != nil {
		fmt.Println("Writing file failed:", err)
		return
	}

	fmt.Printf("JSON data has been saved to: %s\n", jsonfile_name)

	jsonData_new, err := os.ReadFile(*jsonfile_name)
	if err != nil {
		fmt.Println("读取文件失败:", err)
		return
	}
	tmp_obj := make(map[int]map[int]float32)
	err = json.Unmarshal(jsonData_new, &tmp_obj)
	if err != nil {
		fmt.Println("JSON 解码失败:", err)
		return
	}


}

func printSomeInfor() {
	fmt.Println("sequence_count:", sequence_count)
	fmt.Println("failed_count:", failed_count)
	fmt.Printf("sequence average length: %.2f\n", (float64(syscall_count) / float64(sequence_count)))

	//for k, v := range successor_frequece {
	//	fmt.Printf("successor_frequece[%d]: %d\n", k, v)
	//}

	fmt.Printf("len of successor_frequece: %d\n", len(successor_frequece))

	//fmt.Println("successor_frequece[88]:", successor_frequece[88])
	//fmt.Println("successor_Prope[88]:", successor_Prope[88])

	total_successor_count := int32(0)
	for _, v := range successor_frequece {
		for _, v1 := range v {
			total_successor_count += int32(v1)
		}
	}
	fmt.Println("total_successor_count:", total_successor_count)
}

func main() {
	flag.Parse()
	if *syscallSequencePath == "" {
		fmt.Printf("Please specify the path to syscall sequences dir.")
		return
	}
	readSyscallIDs()
	processSyscallSequences()
	calculateSuccessorProbalility()
	saveToJson()

	printSomeInfor()
}
