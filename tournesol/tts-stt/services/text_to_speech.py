from kokoro import KPipeline
from IPython.display import display, Audio
import soundfile as sf
import torch

model_ready = False

def is_tts_model_loaded():
    return model_ready

def load_tts_model():
    global pipeline, model_ready
    try:
        pipeline = KPipeline(lang_code='b')
        model_ready = True
    except Exception as e:
        print(f"Failed to load tts model: {e}")
        model_ready = False

def text_to_speech(prompt: str, filepath):
    if not model_ready or pipeline is None:
        raise RuntimeError("TTS model is not loaded. Call load_tts_model() first.")
    
    generator = pipeline(prompt, voice='af_heart')
    for _, (_, _, audio) in enumerate(generator):
        #print(i, gs, ps)
        #display(Audio(data=audio, rate=24000, autoplay=i==0))
        sf.write(f'{filepath}', audio, 24000)