package migrations

import (
	"github.com/1Panel-dev/1Panel/agent/app/model"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddVllmTables creates the vLLM management tables and seeds a few generic
// launch recipes (the official 1Panel recipe catalog is not open source).
var AddVllmTables = &gormigrate.Migration{
	ID: "20261121-add-vllm-tables",
	Migrate: func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&model.VllmModel{}, &model.VllmRecipe{}, &model.VllmDeployment{}); err != nil {
			return err
		}
		builtin := []model.VllmRecipe{
			{
				Name: "通用 · 单卡 NVIDIA", GPUType: "nvidia", Image: "vllm/vllm-openai:latest", Builtin: true,
				Args:        "--max-model-len 8192\n--gpu-memory-utilization 0.9\n--enable-prefix-caching",
				Description: "适合 7B-14B 稠密模型在单张 24GB+ 显卡上运行",
			},
			{
				Name: "通用 · 多卡张量并行", GPUType: "nvidia", Image: "vllm/vllm-openai:latest", Builtin: true,
				Args:        "--tensor-parallel-size 2\n--max-model-len 16384\n--gpu-memory-utilization 0.9\n--enable-prefix-caching",
				Description: "两张显卡张量并行，按实际卡数修改 --tensor-parallel-size",
			},
			{
				Name: "FP8 量化模型", GPUType: "nvidia", Image: "vllm/vllm-openai:latest", Builtin: true,
				Args:        "--quantization fp8\n--kv-cache-dtype fp8\n--max-model-len 32768\n--gpu-memory-utilization 0.92",
				Description: "FP8 权重 + FP8 KV 缓存，需 Ada / Hopper 及以上架构",
			},
			{
				Name: "工具调用 (Qwen)", GPUType: "nvidia", Image: "vllm/vllm-openai:latest", Builtin: true,
				Args:        "--enable-auto-tool-choice\n--tool-call-parser hermes\n--max-model-len 32768",
				Description: "为 Qwen 系列开启 OpenAI 兼容的 function calling",
			},
			{
				Name: "推理模型 (DeepSeek-R1 蒸馏)", GPUType: "nvidia", Image: "vllm/vllm-openai:latest", Builtin: true,
				Args:        "--reasoning-parser deepseek_r1\n--max-model-len 32768",
				Description: "分离 reasoning_content 与最终回答",
			},
		}
		for i := range builtin {
			var count int64
			tx.Model(&model.VllmRecipe{}).Where("name = ? AND builtin = ?", builtin[i].Name, true).Count(&count)
			if count == 0 {
				if err := tx.Create(&builtin[i]).Error; err != nil {
					return err
				}
			}
		}
		return nil
	},
}
