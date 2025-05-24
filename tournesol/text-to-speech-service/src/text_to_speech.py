from kokoro import KPipeline
from IPython.display import display, Audio
import soundfile as sf
import torch

model_ready = False

def is_model_loaded():
    if is_model_loaded():
        return {"status": "ready"}, 200
    else:
        return {"status": "not ready"}, 503

def load_model():
    global pipeline, model_ready
    try:
        pipeline = KPipeline(lang_code='b')
        model_ready = True
    except Exception as e:
        print(f"Failed to load model: {e}")
        model_ready = False

def text_to_speech(prompt: str, filename):
    generator = pipeline(prompt, voice='af_heart')
    for i, (gs, ps, audio) in enumerate(generator):
        print(i, gs, ps)
        display(Audio(data=audio, rate=24000, autoplay=i==0))
        sf.write(f'../files/sent_audios/{filename}', audio, 24000)