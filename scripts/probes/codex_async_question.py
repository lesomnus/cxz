#!/usr/bin/env python3
"""Opt-in live protocol probe. Uses existing Codex login; never reads credentials."""
import json
import subprocess
import tempfile
import threading
import queue
import time

with tempfile.TemporaryDirectory(prefix="cxz-codex-question-") as workspace:
    proc = subprocess.Popen(["codex", "app-server", "--listen", "stdio://"],
                            cwd=workspace, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, text=True)
    messages = queue.Queue()
    def read():
        for line in proc.stdout:
            messages.put(json.loads(line))
    threading.Thread(target=read, daemon=True).start()
    def send(id, method, params):
        proc.stdin.write(json.dumps(dict(id=id, method=method, params=params)) + "\n")
        proc.stdin.flush()
    def receive(predicate, timeout=90):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            v = messages.get(timeout=max(.1, deadline-time.monotonic()))
            if v.get("error"):
                raise RuntimeError(v["error"])
            if v.get("method") == "item/completed":
                item = v.get("params", {}).get("item", {})
                if item.get("type") == "agentMessage":
                    print(json.dumps({"item": item}, ensure_ascii=False), flush=True)
            if predicate(v):
                return v
        raise TimeoutError("protocol timeout")
    try:
        send("init", "initialize", {"clientInfo":{"name":"cxz-probe","version":"1"},"capabilities":{"experimentalApi":True}})
        receive(lambda v: v.get("id") == "init")
        send("thread", "thread/start", {"cwd":workspace,"sandbox":"read-only","approvalPolicy":"never","ephemeral":True,
             "developerInstructions":"Protocol test only. Do not read files, use shell, browse, or delegate. Only ask the requested question and acknowledge its answer."})
        thread = receive(lambda v: v.get("id") == "thread")["result"]["thread"]["id"]
        send("ask", "turn/start", {"threadId":thread,"input":[{"type":"text","text":"Use request_user_input_async to ask exactly one multiple-choice question: choose a theme, with options Light and Dark. Do not merely print the choices. After posting the question, finish your turn. When I later answer, acknowledge the chosen option in one short sentence."}]})
        question = receive(lambda v: v.get("method") == "item/completed" and bool(v.get("params",{}).get("item",{}).get("questions")))
        receive(lambda v: v.get("method") == "turn/completed")
        send("answer", "turn/start", {"threadId":thread,"input":[],"toolOutput":{"name":"request_user_input_async","output":json.dumps({"answers":[{"question":"choose a theme","answer":"Dark"}]})}})
        receive(lambda v: v.get("method") == "turn/completed")
        print("PROBE_COMPLETE", flush=True)
    finally:
        proc.terminate()
        try: proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
