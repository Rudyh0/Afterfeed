# Text detection model

`ppocr_v3_det.onnx` is the PP-OCRv3 text detection model (`ch_PP-OCRv3_det`)
from [PaddleOCR](https://github.com/PaddlePaddle/PaddleOCR), as converted to
ONNX by [RapidOCR](https://github.com/RapidAI/RapidOCR) (rapidocr_onnxruntime
1.2.3). Both are licensed under the
[Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0).

SHA-256: `3439588c030faea393a54515f51e983d8e155b19a2e8aba7891934c1cf0de526`

It finds where lines of text are in an image. It does not read them;
Tesseract does that.
