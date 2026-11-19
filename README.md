# Tobi Dynavox Spellchecker

A portable Windows 10/11 x64 spelling review tool for people composing their own AAC messages. It uses the installed Windows English (US) spelling dictionary and needs no account, AI model, installer, or internet connection to check a draft. It does not record drafts or send them anywhere. This is an independent tool, not a Tobii Dynavox product.

## Download

- [Download the portable Windows EXE](https://raw.githubusercontent.com/tbenitz/Tobi-Dynavox-Spellchecker/main/docs/downloads/Tobi-Dynavox-Spellchecker.exe)
- [Download the source ZIP](https://raw.githubusercontent.com/tbenitz/Tobi-Dynavox-Spellchecker/main/docs/downloads/Tobi-Dynavox-Spellchecker-source.zip)

## Check and replace a draft

1. Run **Tobi-Dynavox-Spellchecker.exe**. Leave it open while using your AAC app.
2. Focus the speech field and press **Ctrl+Alt+F8**. Or choose **Capture field**, then move the pointer to the speech field within five seconds. The spellchecker minimizes while waiting for the pointer.
3. Read the **Review and edit the correction** box and edit it as needed. Suggestions can get names and specialized words wrong.
4. Choose **Apply to AAC**. It attempts to replace the text in the same field it read. If the draft changed since capture, the tool will not overwrite it. **Undo AAC fix** restores the previous text if the field still contains the correction.

This tool never invokes a Speak button or sends a Speak shortcut. If your AAC app is set to speak as text is inserted, it may announce edits according to its own settings. Direct editing requires the speech field to expose a writable Windows UI Automation value. It treats the field as plain text. Use the AAC app's own editing tools for drafts containing symbols or formatting that you need to preserve.

## If TD Snap does not let another app edit the Speech field

1. In TD Snap, use the Message Bar **Share → Copy** or the **Copy Message Window Text** action.
2. In Spellchecker, choose **Paste draft**, then **Check spelling**. Review the preview and choose **Copy correction**.
3. In TD Snap, confirm the original draft has not changed. Select all text in the Message Window using a button set to **Select Text → Select all**, and use **Paste Clipboard into Message Window**. Some page sets already include a Message Window Editing page with these actions. Verify the resulting text before speaking it. If the page set inserts rather than replaces the selection, use the Message Window editing controls or undo.

The TD Snap manual describes these actions. [TD Snap Message Bar and Button Actions](https://download.mytobiidynavox.com/Snap/documents/TD_Snap_UsersManual_v1-17_en-US_12005874_WEB.pdf)

## Requirements and test status

- Windows 10/11 x64, with the English (US) spelling dictionary installed. If the spelling service reports that the dictionary is missing, add English (United States) in Windows Language settings.
- This tool corrects spelling. It does not rewrite sentence meaning, translate, or apply grammar rules.
- Direct write-back has not been verified on a physical Tobii Dynavox device. Test with a short unsent draft first, such as “I wnat a glass of water.”
- The executable is unsigned. Windows may display its usual warning for a new unsigned app. It does not need administrator privileges.

## Build from source

To build locally with Go 1.25.5 on 64-bit Windows:

```powershell
go test ./...
go build -buildvcs=false -trimpath -ldflags "-s -w -H windowsgui" -o Tobi-Dynavox-Spellchecker.exe .
```

Portable unit tests cover UTF-16 replacement offsets, stale-draft protection, read-back verification and undo. Windows-only tests exercise the system dictionary and initialize the real UI Automation service. Live device testing is still needed for the TD Snap direct route.
