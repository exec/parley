const express = require('express');
const nsfw = require('nsfwjs');
const tf = require('@tensorflow/tfjs-node');
const multer = require('multer');

const app = express();
const upload = multer({
  storage: multer.memoryStorage(),
  limits: { fileSize: 25 * 1024 * 1024 },
});

let model = null;

async function loadModel() {
  model = await nsfw.load();
  console.log('NSFW model loaded');
}

app.post('/check', upload.single('file'), async (req, res) => {
  if (!req.file) {
    return res.status(400).json({ error: 'no file provided' });
  }

  if (!model) {
    // Model still loading — fail open so uploads aren't blocked at startup
    return res.status(503).json({ error: 'model not ready' });
  }

  try {
    const image = await tf.node.decodeImage(req.file.buffer, 3);
    const predictions = await model.classify(image);
    image.dispose();
    res.json({ predictions });
  } catch (err) {
    console.error('NSFW check error:', err.message);
    // Fail open on decode errors (unsupported format, etc.)
    res.status(500).json({ error: err.message });
  }
});

app.get('/health', (req, res) => {
  res.json({ ok: true, model_loaded: model !== null });
});

loadModel()
  .then(() => {
    app.listen(8081, '127.0.0.1', () => {
      console.log('NSFW service listening on 127.0.0.1:8081');
    });
  })
  .catch((err) => {
    console.error('Failed to load NSFW model:', err);
    process.exit(1);
  });
