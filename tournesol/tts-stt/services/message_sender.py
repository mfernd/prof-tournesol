import discord
import asyncio

class VoiceSender(discord.Client):
    def __init__(self, user_id, voice_file_path, result_future, **kwargs):
        super().__init__(**kwargs)
        self.user_id = user_id
        self.voice_file_path = voice_file_path
        self.result_future = result_future

    async def on_ready(self):
        print(f'Logged in as {self.user}')
        user = await self.fetch_user(self.user_id)
        try:
            message_sent = await user.send("Here is your generated voice message 🎧", file=discord.File(self.voice_file_path))
            print("✅ Voice message sent!")
            self.result_future.set_result(message_sent)
        except Exception as e:
            print(f"❌ Failed to send DM: {e}")
            self.result_future.set_exception(e)
        await self.close()

async def send_voice_dm(token, user_id, voice_file_path):
    loop = asyncio.get_event_loop()
    result_future = loop.create_future()

    intents = discord.Intents.default()
    client = VoiceSender(user_id, voice_file_path, result_future, intents=intents)

    await client.start(token)
    return await result_future
