"""Minimal ngspice rawfile reader (standard library only, no numpy).

Usage: python3 -I readraw.py <sim.raw> [--at <t>[,<t>...]] [--all]
       [--svg <out.svg> [--vars <name>[,<name>...]]]

--svg also writes a standalone SVG plot of the first plot (x = first variable,
default traces = all voltage variables; --vars picks names, e.g. v(out)).
The text output is unchanged; "wrote <path>" goes to stderr.

Helper used by the `simulate` step of the TC1 boost demo (see howto/README.md).
It reads the out/sim.raw
written by the ngspice tool profile and prints a header summary plus the
samples nearest to the requested times (default 1,5,6,10,15 ms).
Supports binary and ASCII real rawfiles; only the first plot is read.
"""
import argparse
import math
import struct
import sys
from xml.sax.saxutils import escape

DEFAULT_TIMES = [1e-3, 5e-3, 6e-3, 10e-3, 15e-3]


def fail(msg):
    print("readraw: " + msg, file=sys.stderr)
    sys.exit(2)


def parse(data):
    """Return (header dict, variables, points) for the first plot."""
    bpos, vpos = data.find(b"Binary:\n"), data.find(b"Values:\n")
    cands = [(p, m) for p, m in ((bpos, b"Binary:\n"), (vpos, b"Values:\n")) if p >= 0]
    if not cands:
        fail("no 'Binary:' or 'Values:' section found")
    pos, marker = min(cands)
    binary = marker == b"Binary:\n"
    lines = data[:pos].decode("latin-1").splitlines()
    head, variables, in_vars = {}, [], False
    for ln in lines:
        if in_vars and ln[:1] in ("\t", " "):
            f = ln.split()
            if len(f) < 3:
                fail("bad variable line: %r" % ln)
            variables.append((f[0], f[1], f[2]))
        elif ":" in ln:
            k, v = ln.split(":", 1)
            head[k.strip()] = v.strip()
            in_vars = k.strip() == "Variables"
    try:
        nvars, npts = int(head["No. Variables"]), int(head["No. Points"])
    except (KeyError, ValueError):
        fail("missing or bad 'No. Variables' / 'No. Points'")
    if "complex" in head.get("Flags", "").split():
        fail("complex rawfiles are not supported")
    if len(variables) != nvars:
        fail("variable list has %d entries, expected %d" % (len(variables), nvars))
    body = data[pos + len(marker):]
    need = nvars * npts
    if binary:
        if len(body) < need * 8:
            fail("truncated file: %d bytes of data, need %d" % (len(body), need * 8))
        vals = struct.unpack("<%dd" % need, body[: need * 8])
        rest = body[need * 8:]
    else:
        text = body.decode("latin-1")
        cut = text.find("Title:")
        rest = text[cut:] if cut >= 0 else ""
        toks = (text[:cut] if cut >= 0 else text).split()
        if len(toks) < npts * (nvars + 1):
            fail("truncated file: %d values, need %d" % (len(toks), npts * (nvars + 1)))
        try:
            vals = [float(toks[i * (nvars + 1) + 1 + j]) for i in range(npts) for j in range(nvars)]
        except ValueError:
            fail("bad numeric value in ASCII Values section")
    if rest and (b"Title:" in rest if binary else "Title:" in rest):
        print("note: file holds several plots, reading only the first", file=sys.stderr)
    return head, variables, [vals[i * nvars:(i + 1) * nvars] for i in range(npts)]


def row(variables, pt):
    return "  ".join("%s=%.4g" % (v[1], x) for v, x in zip(variables, pt))


def ticks(lo, hi, n=5):
    """Return 'nice' tick values covering roughly n intervals of [lo, hi]."""
    raw = ((hi - lo) or 1.0) / n
    mag = 10 ** math.floor(math.log10(raw))
    step = min(m * mag for m in (1, 2, 5, 10) if m * mag >= raw)
    k = math.ceil(lo / step - 1e-9)
    out = []
    while k * step <= hi + step * 1e-9:
        out.append(round(k * step, 12))
        k += 1
    return out


def write_svg(path, head, variables, pts, names):
    """Write a standalone SVG plot of the named variables against the first one."""
    sel = [i for i, v in enumerate(variables) if (v[1] in names if names else v[2] == "voltage")]
    if not pts or not sel:
        fail("nothing to plot")
    if len(pts) > 4000:
        pts = pts[:: -(-len(pts) // 4000)]
    xs = [p[0] for p in pts]
    xs_span = max(xs) - min(xs)
    sc, unit = (1e3, "ms") if xs_span < 1 else (1.0, "s")
    ys = [p[i] for p in pts for i in sel]
    x0, x1, y0, y1 = min(xs) * sc, max(xs) * sc, min(ys), max(ys)
    if y0 == y1:
        y0, y1 = y0 - 0.5, y1 + 0.5
    xt, yt = ticks(x0, x1), ticks(y0, y1)
    x0, x1, y0, y1 = min(x0, xt[0]), max(x1, xt[-1]), min(y0, yt[0]), max(y1, yt[-1])
    L, R, T, B, W, H = 70, 150, 40, 50, 900, 450
    px = lambda x: L + (x - x0) / ((x1 - x0) or 1.0) * (W - L - R)
    py = lambda y: H - B - (y - y0) / (y1 - y0) * (H - T - B)
    colors = ["#1f77b4", "#d62728", "#2ca02c", "#ff7f0e", "#9467bd", "#8c564b", "#17becf", "#e377c2"]
    o = ['<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" '
         'font-family="sans-serif" font-size="11">' % (W, H, W, H),
         '<rect width="100%" height="100%" fill="#fff"/>',
         '<text x="%d" y="22" font-size="14" text-anchor="middle">%s</text>' % (W // 2, escape(head.get("Title", "")))]
    for t in xt:
        o.append('<line x1="%.1f" y1="%d" x2="%.1f" y2="%d" stroke="#ddd"/>' % (px(t), T, px(t), H - B))
        o.append('<text x="%.1f" y="%d" text-anchor="middle">%.4g</text>' % (px(t), H - B + 16, t))
    for t in yt:
        o.append('<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#ddd"/>' % (L, py(t), W - R, py(t)))
        o.append('<text x="%d" y="%.1f" text-anchor="end">%.4g</text>' % (L - 6, py(t) + 4, t))
    o.append('<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="#000"/>' % (L, T, W - L - R, H - T - B))
    o.append('<text x="%d" y="%d" text-anchor="middle">%s (%s)</text>'
             % ((L + W - R) // 2, H - 10, escape(variables[0][1]), unit))
    for n, i in enumerate(sel):
        c = colors[n % len(colors)]
        o.append('<polyline fill="none" stroke="%s" stroke-width="1.5" points="%s"/>'
                 % (c, " ".join("%.1f,%.1f" % (px(p[0] * sc), py(p[i])) for p in pts)))
        o.append('<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="2"/>'
                 '<text x="%d" y="%d">%s</text>' % (W - R + 12, T + 12 + 16 * n, W - R + 32, T + 12 + 16 * n,
                                                    c, W - R + 38, T + 16 + 16 * n, escape(variables[i][1])))
    try:
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("\n".join(o) + "\n</svg>\n")
    except OSError as e:
        fail(str(e))
    print("wrote " + path, file=sys.stderr)


def main():
    ap = argparse.ArgumentParser(description="Read an ngspice rawfile.")
    ap.add_argument("raw")
    ap.add_argument("--svg", metavar="PATH", help="write an SVG waveform plot of the first plot")
    ap.add_argument("--vars", help="comma-separated variable names to plot (default: all voltages)")
    ap.add_argument("--at", help="comma-separated times in seconds")
    ap.add_argument("--all", action="store_true", help="print every point")
    args = ap.parse_args()
    try:
        times = [float(t) for t in args.at.split(",")] if args.at else DEFAULT_TIMES
    except ValueError:
        fail("--at expects comma-separated floats")
    try:
        with open(args.raw, "rb") as fh:
            data = fh.read()
    except OSError as e:
        fail(str(e))
    head, variables, pts = parse(data)
    names = args.vars.split(",") if args.vars else []
    for n in names:
        if n not in [v[1] for v in variables]:
            fail("unknown variable " + n)
    for k in ("Title", "Plotname", "Flags", "No. Variables", "No. Points"):
        print("%s: %s" % (k, head.get(k, "")))
    print("Variables:")
    for i, n, t in variables:
        print("  %s  %s  %s" % (i, n, t))
    if args.all:
        for p in pts:
            print(row(variables, p))
    elif pts:
        for t in times:
            print(row(variables, min(pts, key=lambda p: abs(p[0] - t))))
    if args.svg:
        write_svg(args.svg, head, variables, pts, names)


if __name__ == "__main__":
    main()
