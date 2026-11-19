"""Offline inference process for the Windows app. Drafts travel over stdin only."""
import hashlib
import json
import os
import pickle
import re
import secrets
import sys
import zipfile
from pathlib import Path
from urllib.request import urlopen

HUB = {
    "neu-bert": ("murali1996/bert-base-cased-spell-correction", "d2a5bbccc41a0f4ff5e7c16e1c3b8d96ba8883b2",
                 ("config.json", "vocab.txt", "pytorch_model.bin", "tokenizer_config.json", "special_tokens_map.json")),
    "bart": ("oliverguhr/spelling-correction-english-base", "0e3958355a09d2816ed2701fdc2f4471d46c320e",
             ("config.json", "merges.txt", "model.safetensors", "vocab.json", "tokenizer.json", "tokenizer_config.json", "special_tokens_map.json")),
    "t5": ("ai-forever/T5-large-spell", "c32125039b7df52d4d7cda059a0f2f74c29e8e02",
           ("config.json", "spiece.model", "model.safetensors", "tokenizer.json", "tokenizer_config.json", "special_tokens_map.json")),
    "mt5": ("ai-forever/sage-mt5-large", "ab7807052d9728547c97270408cccd9c605af897",
            ("config.json", "spiece.model", "model.safetensors", "generation_config.json", "tokenizer_config.json", "special_tokens_map.json")),
    "tiny": ("torinriley/spell-correct-transformer", "e0bc1879bff283c20fa8a709580b6c281b6b8666",
             ("config.json", "model.safetensors")),
}
VOCAB_DIGEST = "0bdbeed9f99760810b51f27ea609f8f8366c3038b3de99e7603a935018929a41"
DRIVE = {
    "neu-bert": {"vocab.pkl": ("11Bo86aI0MxAU1MHpF-eYfAHg3HqiT9me", VOCAB_DIGEST)},
    "neu-lstm": {
        "vocab.pkl": ("1M7MH3bL0pvnN5OoIBIxZV-F7G-XXi7qU", VOCAB_DIGEST),
        "pytorch_model.bin": ("1jlHASZzwJ7EaaVdtlM2w8p0Zl2KbPHFe", "91e4ad7ff9d8e1524ceb5171de413cfb3c15232ba5ba82ac6e57b4720429d952"),
    },
}

def emit(**msg):
    print(json.dumps(msg, ensure_ascii=True), flush=True)

def digest(path):
    with path.open("rb") as f:
        return hashlib.file_digest(f, "sha256").hexdigest()

def drive_file(path, ident, expected):
    if path.is_file() and digest(path) == expected:
        return
    temp = path.with_name(path.name + "." + secrets.token_hex(4) + ".partial")
    url = f"https://drive.usercontent.google.com/download?id={ident}&export=download&confirm=t"
    try:
        with urlopen(url, timeout=90) as response, temp.open("wb") as output:
            if not response.headers.get("Content-Type", "").startswith("application/octet-stream"):
                raise RuntimeError("The upstream checkpoint download did not return a model file.")
            while chunk := response.read(1024 * 1024):
                output.write(chunk)
        if digest(temp) != expected:
            raise RuntimeError("The upstream checkpoint did not match its expected checksum.")
        temp.replace(path)
    finally:
        temp.unlink(missing_ok=True)

def prepare(engine, root):
    p = root / engine
    p.mkdir(parents=True, exist_ok=True)
    marker = p / ".ready-v1"
    files = HUB.get(engine, (None, None, ()))[2]
    extra = DRIVE.get(engine, {})
    if marker.is_file() and all((p / f).is_file() for f in (*files, *extra)):
        return p
    emit(progress="Downloading selected model. The first check needs internet and may take several minutes.")
    if engine in HUB:
        from huggingface_hub import snapshot_download
        repo, revision, files = HUB[engine]
        snapshot_download(repo, revision=revision, allow_patterns=files, local_dir=p, max_workers=3, token=False)
    for filename, (ident, expected) in extra.items():
        drive_file(p / filename, ident, expected)
    if not all((p / f).is_file() for f in (*files, *extra)):
        raise RuntimeError("One or more checkpoint files could not be downloaded.")
    marker.touch()
    return p

class VocabularyUnpickler(pickle.Unpickler):
    def find_class(self, module, name):
        if module == "collections" and name == "OrderedDict":
            from collections import OrderedDict
            return OrderedDict
        raise pickle.UnpicklingError("Unexpected object in NeuSpell vocabulary")

def vocabulary(path):
    with path.open("rb") as f:
        return VocabularyUnpickler(f).load()

def load_neu(engine, path):
    import torch
    from torch import nn
    v = vocabulary(path / "vocab.pkl")
    idx_to_word = {number: word for word, number in v["token2idx"].items()}
    bad = {v["token2idx"][v[key]] for key in ("pad_token", "unk_token")}
    weights = path / "pytorch_model.bin"
    state = torch.load(weights, map_location="cpu", weights_only=True, mmap=zipfile.is_zipfile(weights))
    size = len(v["token_freq"])
    if engine == "neu-lstm":
        char = v["chartoken2idx"]
        n = len(char)
        unknown = char[v["char_unk_token"]]
        with torch.device("meta"):
            recurrent = nn.LSTM(3*n, 512, 2, batch_first=True, bidirectional=True)
            dense = nn.Linear(1024, size)
        recurrent.load_state_dict({k.removeprefix("lstmmodule."): t for k, t in state.items() if k.startswith("lstmmodule.")}, assign=True)
        dense.load_state_dict({k.removeprefix("dense."): t for k, t in state.items() if k.startswith("dense.")}, assign=True)
        recurrent.eval()
        dense.eval()

        def predict(tokens):
            vectors = torch.zeros((1, len(tokens), 3*n))
            for i, word in enumerate(tokens):
                vectors[0, i, char.get(word[0], unknown)] = 1
                vectors[0, i, 2*n + char.get(word[-1], unknown)] = 1
                for c in word[1:-1]:
                    if c in char:
                        vectors[0, i, n + char[c]] += 1
            return dense(recurrent(vectors)[0])[0].argmax(dim=-1).tolist()
    else:
        from transformers import BertConfig, BertModel, BertTokenizerFast
        cfg = BertConfig.from_pretrained(path, local_files_only=True)
        cfg._attn_implementation = "eager"
        tokenizer = BertTokenizerFast.from_pretrained(path, local_files_only=True)
        # The release contains BERT with the additional NeuSpell vocabulary head.
        # Use the pretrained contextual vectors and mean-pool subwords as upstream does.
        bert_state = {k.removeprefix("bert_model.").removeprefix("bert."): t for k, t in state.items()
                      if not k.startswith(("dense.", "classifier."))}
        if "embeddings.word_embeddings.weight" not in bert_state:
            raise RuntimeError("The NeuSpell BERT checkpoint did not contain the expected encoder.")
        with torch.device("meta"):
            bert = BertModel(cfg, add_pooling_layer="pooler.dense.weight" in bert_state)
            dense = nn.Linear(cfg.hidden_size, size)
        missing, extra = bert.load_state_dict(bert_state, strict=False, assign=True)
        if missing or set(extra) - {"embeddings.position_ids", "embeddings.token_type_ids"}:
            raise RuntimeError("The NeuSpell BERT checkpoint does not match the encoder.")
        # Older checkpoints omit these nonpersistent buffers. Recreate them on CPU.
        bert.embeddings.position_ids = torch.arange(cfg.max_position_embeddings).unsqueeze(0)
        bert.embeddings.token_type_ids = torch.zeros_like(bert.embeddings.position_ids)
        dense.load_state_dict({"weight": state["classifier.weight"], "bias": state["classifier.bias"]}, assign=True)
        bert.eval()
        dense.eval()

        def predict(tokens):
            inp = tokenizer(tokens, is_split_into_words=True, return_tensors="pt",
                            max_length=cfg.max_position_embeddings, truncation=True)
            groups = [[] for _ in tokens]
            for i, word in enumerate(inp.word_ids()):
                if word is not None:
                    groups[word].append(i)
            if not all(groups):
                raise ValueError("This line is too long for NeuSpell BERT. Try a shorter draft.")
            vectors = bert(**inp).last_hidden_state[0]
            merged = torch.stack([vectors[group].mean(dim=0) for group in groups])
            return dense(merged).argmax(dim=-1).tolist()

    def run(text):
        spans = list(re.finditer(r"[^\W_]+|[^\w\s]", text, re.UNICODE))
        if not spans:
            return text
        if len(spans) > 240:
            raise ValueError("This line is too long for NeuSpell. Try a shorter draft.")
        original = [m.group() for m in spans]
        result, last = [], 0
        for match, key in zip(spans, predict(original), strict=True):
            raw = match.group()
            guess = idx_to_word.get(key, raw) if key not in bad else raw
            if not raw.isalpha() or not guess.isalpha():
                guess = raw
            result.extend((text[last:match.start()], guess))
            last = match.end()
        result.append(text[last:])
        return "".join(result)

    return run

def load_seq(path):
    from transformers import AutoModelForSeq2SeqLM, AutoTokenizer
    tokenizer = AutoTokenizer.from_pretrained(path, local_files_only=True, use_fast=path.name != "mt5")
    model = AutoModelForSeq2SeqLM.from_pretrained(path, local_files_only=True,
                                                  use_safetensors=True, low_cpu_mem_usage=True).eval()

    def run(text):
        inp = tokenizer(text, return_tensors="pt")
        n = inp["input_ids"].shape[1]
        if n > 448:
            raise ValueError("This line is too long for the selected model. Try a shorter draft.")
        limit = min(512, max(32, n * 2 + 12))
        output = model.generate(**inp, do_sample=False, num_beams=1, max_new_tokens=limit)
        if output.shape[1] >= limit + 1:
            raise ValueError("The selected model could not finish this line. Try a shorter draft.")
        return tokenizer.decode(output[0], skip_special_tokens=True)

    return run

def load_tiny(path):
    import torch
    from safetensors.torch import load_file
    from tiny_model import ModelConfig, SpellCorrectorTransformer
    from vocab import BOS, EOS, PAD, decode, encode
    cfg = ModelConfig(**json.loads((path / "config.json").read_text(encoding="utf-8")))
    model = SpellCorrectorTransformer(cfg)
    model.load_state_dict(load_file(path / "model.safetensors"))
    model.eval()

    def one(word):
        src = torch.tensor([encode(word.lower())])
        src_mask = src == PAD
        ys = torch.tensor([[BOS]])
        for _ in range(min(24, cfg.max_len - 1)):
            logits = model(src, ys, src_mask, ys == PAD)
            next_id = logits[:, -1, :].argmax(-1)
            ys = torch.cat([ys, next_id.unsqueeze(1)], dim=1)
            if next_id.item() == EOS:
                break
        guess = decode(ys[0].tolist()[1:])
        if not guess or not (word.islower() or word.istitle() or word.isupper()):
            return word
        return guess.upper() if word.isupper() else guess.title() if word.istitle() else guess

    def run(text):
        cache = {}
        def replace(m):
            word = m.group()
            if word not in cache:
                cache[word] = one(word)
            return cache[word]
        return re.sub(r"(?<![\w'’])[A-Za-z]{3,22}(?![\w'’])", replace, text)
    return run

def map_lines(draft, correct):
    parts = re.split(r"(\r\n|\r|\n)", draft)
    for i in range(0, len(parts), 2):
        line = parts[i]
        stripped = line.strip()
        if stripped:
            start = len(line) - len(line.lstrip())
            end = len(line.rstrip())
            fixed = correct(stripped).strip()
            if not fixed.strip() or "\n" in fixed or "\r" in fixed:
                raise ValueError("The selected model returned an invalid line. Your draft was not changed.")
            parts[i] = line[:start] + fixed + line[end:]
    return "".join(parts)

def main():
    engine, root = sys.argv[1:]
    if engine not in HUB and engine not in DRIVE:
        raise ValueError("Unknown spelling model.")
    path = prepare(engine, Path(root))
    emit(progress="Loading the selected model into memory…")
    import torch
    torch.set_num_threads(min(4, os.cpu_count() or 1))
    torch.set_num_interop_threads(1)
    draft = json.load(sys.stdin)["text"]
    with torch.inference_mode():
        if engine.startswith("neu-"):
            correct = load_neu(engine, path)
        elif engine == "tiny":
            correct = load_tiny(path)
        else:
            correct = load_seq(path)
        emit(progress="Checking the draft on this computer…")
        emit(text=map_lines(draft, correct))

if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        # Do not surface arbitrary exception text or draft content in logs/status.
        message = str(exc) if isinstance(exc, ValueError) and str(exc).startswith(("This line", "The selected model")) else ""
        emit(error=message or f"Model setup or spellcheck failed ({type(exc).__name__}). Check internet/free disk space for first use, or select Windows Spell Checking.")
        sys.exit(1)
