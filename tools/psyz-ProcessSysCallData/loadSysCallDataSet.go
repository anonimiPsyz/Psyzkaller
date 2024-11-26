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
	syscallIDPath       = flag.String("s", "syscallIDs.txt", "file of syscall ID/name pairs.")                 //系统调用名和ID的对应关系，通过genSyscallIDs.sh脚本生成
	syscallSequencePath = flag.String("d", "", "path to DongTing database")                                    //系统调用序列数据集的路径，文件夹中每个文件对应一条调用序列，用系统调用的名字来保存的
	jsonfile_name       = flag.String("o", "successor_Prope.json", "directory to store deserialized programs") //保存syscall直接后继的概率信息，里面是用syzkaller使用的ID来表示每个系统调用
)

// var syscallSequencePath = "/home/fuyu/AllOfSyz/ngram_DongTing/testCode/ProcessSysCallData/Normal_data"

var syscall_name_2_id = make(map[string]int)
var syscall_id_2_name = make(map[int]string)

//var syscall_sequences = make([][]int, 0)

var successor_frequece = make(map[int]map[int]int32)
var successor_Prope = make(map[int]map[int]float32)

var sequence_count = 0
var syscall_count = 0
var failed_count = 0
var current_count = 0

// 读取syscallIDPath文件，获取系统调用名和ID的对应关系
func readSyscallIDs() {
	// 打开文件
	file, err := os.Open(*syscallIDPath)
	if err != nil {
		fmt.Println("Can not open file：", err)
		return
	}
	defer file.Close()

	// 创建一个Scanner来读取文件内容
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// 在这里处理每一行的数据，例如打印或进行其他操作
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

	/*
		keys := make([]int, 0, len(syscall_id_2_name))
		for key := range syscall_id_2_name {
			keys = append(keys, key)
		}
		sort.Ints(keys)
		// 按照键的大小顺序打印 map 中的元素
		for _, key := range keys {
			fmt.Printf("Key: %d, Value: %s\n", key, syscall_id_2_name[key])
		}
	*/

	//fmt.Println("length of syscall_name_2_id: ", len(syscall_name_2_id))
	//fmt.Println("length of syscall_id_2_name: ", len(syscall_id_2_name))

	if err := scanner.Err(); err != nil {
		fmt.Println("Error while read file:", err)
	}
}

// 统计系统调用序列中，不同syscall对应后继syscall的出现次数
func calculateSuccessorFrequency(seq []int) {
	for i := 0; i < len(seq)-1; i++ {
		if _, ok := successor_frequece[seq[i]]; !ok {
			successor_frequece[seq[i]] = make(map[int]int32)
		}
		successor_frequece[seq[i]][seq[i+1]]++
	}

}

// 计算系统调用序列中，不同syscall对应后继syscall的出现概率
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

// 读取一个文件，其中包含一条系统调用序列，转化并处理其中的信息
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
	// 创建一个Scanner来读取文件内容
	scanner := bufio.NewScanner(file)
	// 设置缓冲区大小为特大值
	//	const maxCapacity = 512 * 1024 * 1024 // 512MB
	//	buf := make([]byte, maxCapacity)
	//	scanner.Buffer(buf, maxCapacity)

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

// 处理syscallSequencePath下的所有文件的信息
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

// 将处理后的信息保存入json文件
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

	// 读取JSON文件
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

	//fmt.Println("从 JSON 文件中读取的对象，长度为：", len(tmp_obj))
	//fmt.Println("第一个元素的长度为：", len(tmp_obj[0]))
	//fmt.Println("原对象长度为：", len(successor_Prope))
	//fmt.Println("原对象第一个元素的长度为：", len(successor_Prope[0]))

}

// 打印些数据，方便我确认编程时一些数据的处理是否正确
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
