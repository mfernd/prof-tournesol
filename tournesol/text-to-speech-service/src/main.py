from fastapi import FastAPI, File, UploadFile
from dotenv import load_dotenv

import os

from text_to_speech import is_model_loaded, load_model, text_to_speech
from speech_to_text import speech_to_text
from message_sender import send_voice_dm

from models.models import Text

app = FastAPI()
load_dotenv()

TOKEN = os.getenv("TOKEN")
USER_ID = int(os.getenv("USER_ID", 0))

VOICE_FILE_PATH = 'recording1.ogg'

@app.on_event("startup")
def startup():
    load_model()

@app.get("/")
def root():
    return {"Hello": "World"}

@app.get("/healthz")
def health_check():
    return {"status": "ok"}

@app.get("/readyz")
def readiness_check():
    return is_model_loaded()

@app.post("/tts")
async def tts(body: Text):
    text_to_speech(body.prompt, VOICE_FILE_PATH)

    if os.path.exists(VOICE_FILE_PATH):
        await send_voice_dm(TOKEN, USER_ID, f'./recordings/{VOICE_FILE_PATH}')
    else:
        raise FileNotFoundError(f"Voice file {VOICE_FILE_PATH} was not created successfully.")

@app.post("/webhook")
async def webhook(file: UploadFile = File(...)):
    contents = await file.read()
    print(f"Received file {file.filename} of size {len(contents)} bytes")
    with open(f"../files/received_audios/{file.filename}", "wb") as f:
        f.write(contents)

    #speech_to_text(f'../files/received_audios/{file.filename}')

    # api_url = "http://example.com/your-api-endpoint"  # Replace with actual API URL
    # api_response = requests.post(api_url, json={"text": transcribed_text})
    # if api_response.status_code == 200:
    #     print(f"API Response: {api_response.json()}")
    # else:
    #     print(f"Failed to send text to API: {api_response.status_code}, {api_response.text}")

    return {"status": "file received"}
