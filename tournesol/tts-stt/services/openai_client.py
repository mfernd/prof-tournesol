from fastapi.exceptions import HTTPException
import logging
from openai import AsyncOpenAI

async def prompt_ai(prompt: str, OPENAI_URL: str):
    client = AsyncOpenAI(api_key="ignored", base_url=OPENAI_URL)

    try:
        response = await client.chat.completions.create(
            model="gemma3-1b-cpu",
            messages=[
                {
                    "role": "user",
                    "content": f"We have an issue in our kubernetes cluster. Use the follwing message to determine if the person is going to handle the issue. Answer only with Yes or No. Here is the input: '{prompt}'."
                }
            ]
        )
        return response.choices[0].message.content.strip()
    except Exception as e:
        logging.exception("An error occurred while calling the AI.")
        raise HTTPException(status_code=500, detail="Failed to process AI response.")

