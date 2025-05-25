import torch
from transformers import AutoModelForSpeechSeq2Seq, AutoProcessor, pipeline

device = "cpu"
torch_dtype = torch.float32
model_id = "openai/whisper-base.en"

model_ready = False
asr_pipeline = None  # Global pipeline

def is_stt_model_loaded():
    return model_ready

def load_stt_model():
    global model_ready, asr_pipeline
    try:
        model = AutoModelForSpeechSeq2Seq.from_pretrained(
            model_id, torch_dtype=torch_dtype, low_cpu_mem_usage=True, use_safetensors=True
        )
        model.to(device)

        processor = AutoProcessor.from_pretrained(model_id)

        asr_pipeline = pipeline(
            "automatic-speech-recognition",
            model=model,
            tokenizer=processor.tokenizer,
            feature_extractor=processor.feature_extractor,
            torch_dtype=torch_dtype,
            device=0 if device == "cuda" else -1,
        )

        model_ready = True
    except Exception as e:
        print(f"❌ Failed to load STT model: {e}")
        model_ready = False

def speech_to_text(filepath: str):
    if not model_ready or asr_pipeline is None:
        raise RuntimeError("STT model is not loaded. Call load_stt_model() first.")
    
    result = asr_pipeline(filepath)
    return result["text"]
