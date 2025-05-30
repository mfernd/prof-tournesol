import requests
from fastapi.exceptions import HTTPException
from fastapi import FastAPI
from dotenv import load_dotenv
from fastapi import UploadFile, File, Form
from datetime import datetime

import os
import logging

from services.text_to_speech import is_tts_model_loaded, load_tts_model, text_to_speech
from services.speech_to_text import is_stt_model_loaded, load_stt_model, speech_to_text
from services.message_sender import send_voice_dm
from services.openai_client import prompt_ai

from models.TextModel import Text

app = FastAPI()
load_dotenv()

TOKEN = os.getenv("TOKEN")
USER_ID = int(os.getenv("USER_ID", 0))
OPENAI_URL = os.getenv("OPENAI_URL", 'http://localhost:8008/v1/')
RECEIVED_FILE_PATH = 'files/received_audios'
SENT_FILE_PATH = 'files/sent_audios'
VOICE_FILE = 'recording'

conversations = {}

@app.on_event("startup")
def startup():
    print(" --- Loading Text-to-Speech model... --- ")
    load_tts_model()
    print(" --- Loading Speech-to-Text model... ---")
    load_stt_model()

@app.get("/")
def root():
    return {"status": 200, "message": "Hello World"}

@app.get("/healthz")
def health_check():
    return {"status": 200, "message": "OK"}

@app.get("/readyz")
def readiness_check():
    if is_tts_model_loaded() and is_stt_model_loaded():
        return {"status": 200, "message": "OK"}
    else:
        return {"status": 500, "message": "Loading"}

@app.post("/tts")
async def tts(body: Text):

    filepath = f'{SENT_FILE_PATH}/{VOICE_FILE}.ogg'

    try:
        text_to_speech(body.prompt, filepath)
    except Exception as e:
        logging.exception("Text-to-speech generation failed.")
        raise HTTPException(status_code=500, detail="Failed to generate voice file.")

    # Check file existence
    if not os.path.exists(filepath):
        logging.error(f"Voice file not found at expected path: {filepath}")
        raise HTTPException(status_code=500, detail="Voice file was not created successfully.")
    
    # Send the file
    try:
        message_sent = await send_voice_dm(TOKEN, USER_ID, filepath)
    except Exception as e:
        logging.exception("Failed to send voice DM.")
        raise HTTPException(status_code=500, detail="Failed to send voice message.")

    # Example of the conversations dictionary:
    # conversations = {
    #     "13263890": {
    #         "ai_message": "There is a issue with pod 'nginx', out of memory",
    #         "user_reply": "I can handle it",
    #         "user_answer": "Yes"
    #     }

    conversations[str(message_sent.id)] = {"ai_message": body.prompt, "user_reply": "None", "user_answer": "None"}

    return {"status": 201, "message": "Created"}

@app.post("/webhook")
async def webhook(file: UploadFile = File(...), author: str = Form(...), reference_id: str = Form(...)):
    contents = await file.read()
    print(f"Received file {file.filename} of size {len(contents)} bytes")
    with open(f'{RECEIVED_FILE_PATH}/{file.filename}', 'wb') as f:
        f.write(contents)

    user_transcript = speech_to_text(f'{RECEIVED_FILE_PATH}/{file.filename}')

    conversations[reference_id]["user_reply"] = user_transcript

    try:
        result = await prompt_ai(user_transcript, OPENAI_URL)
        conversations[reference_id]["user_answer"] = result
        print(conversations)
    except HTTPException as e:
        raise e

    with open(f'files/conversation_logs.txt', 'a') as f:
        f.write("------------------------------\n")
        f.write(f"Timestamp: {datetime.now()}\n")
        f.write(f"Reference ID: {reference_id}\n")
        f.write(f"AI Message: {conversations[reference_id]['ai_message']}\n")
        f.write(f"User Reply: {conversations[reference_id]['user_reply']}\n")
        f.write(f"User Answer: {conversations[reference_id]['user_answer']}\n")

    return {"status": 200, "message": "File received and parsed"}
