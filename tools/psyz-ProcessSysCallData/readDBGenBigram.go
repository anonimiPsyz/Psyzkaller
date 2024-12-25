package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/syzkaller/pkg/db"
	"github.com/google/syzkaller/prog"
	_ "github.com/google/syzkaller/sys"
)

var CorpusSuccessorFrequece = make(map[int]map[int]int32)
var CorpusSuccessorPrope = make(map[int]map[int]float32)

var (
	flagOS        = flag.String("os", runtime.GOOS, "target os")
	flagArch      = flag.String("arch", runtime.GOARCH, "target arch")
	flagCorpus    = flag.String("corpus", "", "name of the corpus file")
	jsonfile_name = flag.String("o", "DTNormalSuccPrope.json", "directory to store deserialized programs") //保存syscall直接后继的概率信息，里面是用syzkaller使用的ID来表示每个系统调用
	flagCorpusDir = flag.String("corpus_dir", "", "directory to store deserialized programs")
)

func calculateCorpusSuccessorProbalility() {
	for i, v0 := range CorpusSuccessorFrequece {
		var fenmu float32
		fenmu = 0
		for _, v2 := range v0 {
			fenmu += float32(v2)
		}
		for j, v1 := range v0 {
			if CorpusSuccessorPrope[i] == nil {
				CorpusSuccessorPrope[i] = make(map[int]float32)
			}
			v1f := float32(v1)
			CorpusSuccessorPrope[i][j] = v1f / fenmu
		}
	}
}
func saveToJson() {
	jsonData, err := json.MarshalIndent(CorpusSuccessorPrope, "", "    ")
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

	fmt.Printf("JSON data has been saved to: %s\n", *jsonfile_name)

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

func processCorpusDB(corpus []*prog.Prog) {
	for _, v := range corpus {
		callerId := -100
		for _, v1 := range v.Calls {
			//fmt.Printf("序号%d ,ID:%v, Name:%v\n", j, v1.Meta.ID, v1.Meta.Name)
			if callerId != -100 {
				if _, ok := CorpusSuccessorFrequece[callerId]; !ok {
					CorpusSuccessorFrequece[callerId] = make(map[int]int32)
				}
				CorpusSuccessorFrequece[callerId][v1.Meta.ID]++
			}
			callerId = v1.Meta.ID
		}
		//fmt.Printf("\n")
	}
	fmt.Println("length of corpus:", len(corpus))
}

func readAndprocessCorpusDB(corpusString string, target *prog.Target) {
	corpus, err := db.ReadCorpus(corpusString, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read corpus: %v\n", err)
		os.Exit(1)
	}
	processCorpusDB(corpus)
}

func readAndprocessCorpusDBDir(corpusDir *string, target *prog.Target) {
	// 遍历目录中的所有文件
	fmt.Printf("遍历目录中的所有文件：%s\n", corpusDir)
	err := filepath.Walk(*corpusDir, func(path string, info os.FileInfo, err error) error {
		fmt.Printf("正在处理文件：%s\n", path)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// 对每个文件调用 readAndprocessCorpusDB
			readAndprocessCorpusDB(path, target)
			return nil
		}
		return nil
	})
	if err != nil {
		fmt.Errorf("error walking the path %q: %v", *corpusDir, err)
	}
}

func main() {
	flag.Parse()
	target, err := prog.GetTarget(*flagOS, *flagArch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if *flagCorpus != "" {
		readAndprocessCorpusDB(*flagCorpus, target)
	} else if *flagCorpusDir != "" {
		readAndprocessCorpusDBDir(flagCorpusDir, target)
	}
	calculateCorpusSuccessorProbalility()
	saveToJson()

}
