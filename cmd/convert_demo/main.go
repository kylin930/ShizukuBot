// ============================================================
// 演示：Go 代码 → 流程图插件 转换
// ============================================================

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"shizukubot/plugin"
)

func main() {
	fmt.Println("========== Go 代码 → 流程图插件 转换演示 ==========")
	fmt.Println()

	// 1. 扫描单文件
	fmt.Println("【步骤1】扫描 main.go 文件...")
	result1, err := plugin.ScanGoSource(filepath.Join("data", "ccb_plus_go", "main.go"))
	if err != nil {
		fmt.Println("  错误:", err)
	} else {
		fmt.Printf("  ✓ 扫描到: %d 个事件, %d 个函数\n", len(result1.Events), len(result1.Functions))
		for _, fn := range result1.Functions {
			fmt.Printf("    - %s (%s)\n", fn.Name, fn.Title)
		}
	}
	fmt.Println()

	// 2. 扫描整个目录（多文件）
	fmt.Println("【步骤2】扫描 ccb_plus_go 目录（多文件）...")
	result, err := plugin.ScanGoDirectory(filepath.Join("data", "ccb_plus_go"))
	if err != nil {
		fmt.Println("  错误:", err)
		return
	}

	fmt.Printf("  ✓ 插件元信息: ID=%s, Name=%s\n", result.PluginMeta.ID, result.PluginMeta.Name)
	fmt.Printf("  ✓ 扫描到: %d 个事件, %d 个函数\n", len(result.Events), len(result.Functions))
	for _, fn := range result.Functions {
		fmt.Printf("    - %s (%s) [%s]\n", fn.Name, fn.Title, fn.Category)
	}
	fmt.Println()

	// 3. 转成插件 JSON
	fmt.Println("【步骤3】转换成流程图插件 JSON...")
	pluginJSON, err := result.ToPluginJSON()
	if err != nil {
		fmt.Println("  错误:", err)
		return
	}

	// 输出 JSON 到文件
	outputPath := filepath.Join("data", "plugins", "plugin_ccb_plus_from_code.json")
	data, _ := json.MarshalIndent(pluginJSON, "", "  ")
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Println("  写入文件错误:", err)
		return
	}

	fmt.Printf("  ✓ 已生成插件 JSON: %s\n", outputPath)
	fmt.Printf("  ✓ 插件大小: %d 字节\n", len(data))
	fmt.Println()

	// 4. 展示生成的内容摘要
	fmt.Println("【生成内容摘要】")
	fmt.Printf("  插件ID: %s\n", pluginJSON["id"])
	fmt.Printf("  插件名称: %s\n", pluginJSON["name"])
	if nodes, ok := pluginJSON["custom_nodes"].([]interface{}); ok {
		fmt.Printf("  自定义节点数: %d\n", len(nodes))
		for i, n := range nodes {
			if m, ok := n.(map[string]interface{}); ok {
				fmt.Printf("    %d. %s (%s)\n", i+1, m["title"], m["type"])
			}
		}
	}
	if graph, ok := pluginJSON["graph"].(map[string]interface{}); ok {
		if nodes, ok := graph["nodes"].([]interface{}); ok {
			fmt.Printf("  流程图节点数: %d\n", len(nodes))
		}
	}
	fmt.Println()

	// 5. 反向转换：流程图 → Go 代码
	fmt.Println("【步骤4】反向转换：流程图 JSON → Go 代码骨架...")
	code, err := plugin.GraphToCode(pluginJSON)
	if err != nil {
		fmt.Println("  错误:", err)
		return
	}

	codeOutputPath := filepath.Join("data", "ccb_plus_go", "generated_from_graph.go")
	if err := os.WriteFile(codeOutputPath, []byte(code), 0644); err != nil {
		fmt.Println("  写入文件错误:", err)
		return
	}

	fmt.Printf("  ✓ 已生成 Go 代码骨架: %s\n", codeOutputPath)
	fmt.Printf("  ✓ 代码大小: %d 字节\n", len(code))
	fmt.Println()

	fmt.Println("========== 转换完成！==========")
	fmt.Println()
	fmt.Println("使用方式：")
	fmt.Println("  1. 将生成的 plugin_ccb_plus_from_code.json 放到 data/plugins/ 目录")
	fmt.Println("  2. 在 WebUI 中刷新插件列表即可看到")
	fmt.Println("  3. 可以继续在流程图编辑器中拖拽连接节点")
}
