package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
)

var (
	// 定义命令行参数
	file1Path  = flag.String("file1", "", "Path to the first JSON file")
	file2Path  = flag.String("file2", "", "Path to the second JSON file")
	factor1    = flag.Float64("factor1", 1, "Factor for the first JSON file")
	factor2    = flag.Float64("factor2", 1, "Factor for the second JSON file")
	outputPath = flag.String("output", "result.json", "Path to the output JSON file")
)

func printTypeInfo(v interface{}) {
	fmt.Printf("Type of variable: %s\n", reflect.TypeOf(v))
}
func readJSONFile(filePath string) (map[int]map[int]float32, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	result := make(map[int]map[int]float32)
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func multiplyValues(dict map[int]map[int]float32, factor float64) {
	for outerKey, innerMap := range dict {
		for innerKey, value := range innerMap {
			dict[outerKey][innerKey] = value * float32(factor)
		}
	}
}

func addDictionaries(dict1, dict2 map[int]map[int]float32) map[int]map[int]float32 {
	result := make(map[int]map[int]float32)

	// Add values from dict1
	for outerKey, innerMap := range dict1 {
		if result[outerKey] == nil {
			result[outerKey] = make(map[int]float32)
		}
		for innerKey, value := range innerMap {
			result[outerKey][innerKey] = value
		}
	}

	// Add values from dict2
	for outerKey, innerMap := range dict2 {
		if result[outerKey] == nil {
			result[outerKey] = make(map[int]float32)
		}
		for innerKey, value := range innerMap {
			result[outerKey][innerKey] += value
		}
	}

	return result
}

func normalizeDict(dict map[int]map[int]float32) map[int]map[int]float32 {
	for outerKey, innerMap := range dict {
		total := float32(0)
		for innerKey, _ := range innerMap {
			total += dict[outerKey][innerKey]
		}
		for innerKey, _ := range innerMap {
			dict[outerKey][innerKey] /= total
		}
	}
	return dict
}

func saveJSONToFile(data map[int]map[int]float32, filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // 设置缩进以便更易读
	return encoder.Encode(data)
}

func main() {

	// 解析命令行参数
	flag.Parse()

	// 检查参数是否提供
	if *file1Path == "" || *file2Path == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	// 读取并解析第一个 JSON 文件
	dict1, err := readJSONFile(*file1Path)
	if err != nil {
		log.Fatalf("Error reading %s: %v", *file1Path, err)
	}
	//fmt.Printf("%v\n", dict1[64])
	multiplyValues(dict1, *factor1)
	//fmt.Printf("%v %v\n", *factor1, dict1[64])

	// 读取并解析第二个 JSON 文件
	dict2, err := readJSONFile(*file2Path)
	if err != nil {
		log.Fatalf("Error reading %s: %v", *file2Path, err)
	}
	//fmt.Printf("%v\n", dict2[64])
	multiplyValues(dict2, *factor2)
	//fmt.Printf("%v %v\n", *factor2, dict2[64])

	resultDict := addDictionaries(dict1, dict2)
	resultDict = normalizeDict(resultDict)

	// 保存 normalizedDict 到 JSON 文件
	err = saveJSONToFile(resultDict, *outputPath)
	if err != nil {
		log.Fatalf("Error saving JSON to file: %v", err)
	}

	fmt.Printf("Normalized JSON saved to %s\n", *outputPath)

}
