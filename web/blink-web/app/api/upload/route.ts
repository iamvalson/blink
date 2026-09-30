import { writeFile } from "fs/promises";
import { join } from "path";

export async function POST(request: Request) {
  const data = await request.formData();
  const file: File | null = data.get("file") as unknown as File;

  if (!file) {
    return new Response("No file uploaded", { status: 400 });
  }

  const bytes = await file.arrayBuffer();
  const buffer = Buffer.from(bytes);

  const filename = `${crypto.randomUUID()}-${file.name.replace(/[^a-zA-Z0-9.]/g, "")}`;
  const path = join(process.cwd(), "public", filename);

  await writeFile(path, buffer);

  // Return the full ngrok URL or localhost URL
  const host = request.headers.get("host") || "localhost:3000";
  const protocol = request.headers.get("x-forwarded-proto") || "http";

  return new Response(
    JSON.stringify({ url: `${protocol}://${host}/${filename}` }),
    {
      headers: { "Content-Type": "application/json" },
    },
  );
}
