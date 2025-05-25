from openai import OpenAI

async def prompt_ai(prompt: str, OPENAI_URL: str):
    client = OpenAI(api_key="ignored", base_url=OPENAI_URL)

    response = await client.responses.create({
        'model': 'gemma3-1b-cpu',
        'input': f'We have an issue in our kubernetes cluster, determine with the following message if the person is going to take care of it. Answer with only True or False. Message: {prompt}'
    })
    return response.get('result', False)