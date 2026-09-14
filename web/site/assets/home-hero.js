(() => {
  const canvas = document.querySelector("[data-hs-net]") || document.querySelector(".hs-net");
  if (!canvas || !canvas.getContext) return;

  const reduced =
    window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const ctx = canvas.getContext("2d", { alpha: true });
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  let nodes = [];
  let W = 0;
  let H = 0;
  let mx = -9999;
  let my = -9999;
  let rafNet = 0;
  let last = 0;

  function resize() {
    W = Math.max(1, window.innerWidth || document.documentElement.clientWidth);
    H = Math.max(1, window.innerHeight || document.documentElement.clientHeight);
    canvas.width = Math.floor(W * dpr);
    canvas.height = Math.floor(H * dpr);
    canvas.style.width = `${W}px`;
    canvas.style.height = `${H}px`;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    seed();
  }

  function seed() {
    const area = W * H;
    const count = Math.max(36, Math.min(90, Math.floor(area / 16000)));
    nodes = Array.from({ length: count }, () => ({
      x: Math.random() * W,
      y: Math.random() * H,
      vx: (Math.random() - 0.5) * 0.22,
      vy: (Math.random() - 0.5) * 0.22,
      r: 1 + Math.random() * 1.6,
      pulse: Math.random() * Math.PI * 2,
    }));
  }

  function step(ts) {
    const dt = Math.min(32, ts - (last || ts)) / 16.67;
    last = ts;
    ctx.clearRect(0, 0, W, H);

    const linkDist = Math.min(150, Math.max(100, Math.min(W, H) * 0.2));
    const linkDist2 = linkDist * linkDist;

    for (const n of nodes) {
      if (!reduced) {
        n.x += n.vx * dt;
        n.y += n.vy * dt;
        n.pulse += 0.025 * dt;
        if (n.x < -20) n.x = W + 20;
        if (n.x > W + 20) n.x = -20;
        if (n.y < -20) n.y = H + 20;
        if (n.y > H + 20) n.y = -20;

        const dx = n.x - mx;
        const dy = n.y - my;
        const d2 = dx * dx + dy * dy;
        if (d2 < 14000 && d2 > 1) {
          const f = 0.014 / Math.sqrt(d2);
          n.vx += dx * f;
          n.vy += dy * f;
        }
        n.vx *= 0.996;
        n.vy *= 0.996;
        const sp = Math.hypot(n.vx, n.vy);
        if (sp > 0.45) {
          n.vx *= 0.45 / sp;
          n.vy *= 0.45 / sp;
        }
      }
    }

    for (let i = 0; i < nodes.length; i++) {
      const a = nodes[i];
      for (let j = i + 1; j < nodes.length; j++) {
        const b = nodes[j];
        const dx = a.x - b.x;
        const dy = a.y - b.y;
        const d2 = dx * dx + dy * dy;
        if (d2 > linkDist2) continue;
        const t = 1 - Math.sqrt(d2) / linkDist;
        const nearMouse =
          (a.x - mx) * (a.x - mx) + (a.y - my) * (a.y - my) < 20000 ||
          (b.x - mx) * (b.x - mx) + (b.y - my) * (b.y - my) < 20000;
        ctx.beginPath();
        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);
        ctx.strokeStyle = nearMouse
          ? `rgba(125, 220, 255, ${0.14 + t * 0.42})`
          : `rgba(77, 228, 255, ${0.04 + t * 0.22})`;
        ctx.lineWidth = nearMouse ? 1.1 : 0.75;
        ctx.stroke();

        if (nearMouse && t > 0.58 && !reduced) {
          const u = (Math.sin(ts * 0.0038 + i + j) + 1) * 0.5;
          ctx.beginPath();
          ctx.arc(a.x + (b.x - a.x) * u, a.y + (b.y - a.y) * u, 1.2, 0, Math.PI * 2);
          ctx.fillStyle = "rgba(220, 245, 255, 0.85)";
          ctx.fill();
        }
      }
    }

    for (const n of nodes) {
      const glow = 0.5 + Math.sin(n.pulse) * 0.22;
      const near = (n.x - mx) * (n.x - mx) + (n.y - my) * (n.y - my) < 16000;
      ctx.beginPath();
      ctx.arc(n.x, n.y, n.r + (near ? 1 : 0), 0, Math.PI * 2);
      ctx.fillStyle = near
        ? `rgba(125, 220, 255, ${0.55 + glow * 0.3})`
        : `rgba(77, 228, 255, ${0.28 + glow * 0.32})`;
      ctx.fill();
    }

    rafNet = requestAnimationFrame(step);
  }

  window.addEventListener(
    "pointermove",
    (e) => {
      mx = e.clientX;
      my = e.clientY;
    },
    { passive: true }
  );
  window.addEventListener(
    "pointerleave",
    () => {
      mx = -9999;
      my = -9999;
    },
    { passive: true }
  );
  window.addEventListener("resize", resize, { passive: true });

  resize();
  rafNet = requestAnimationFrame(step);
  window.addEventListener(
    "pagehide",
    () => cancelAnimationFrame(rafNet),
    { once: true }
  );
})();
