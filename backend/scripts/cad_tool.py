import sys
import os
import json
import ezdxf
from ezdxf.addons.drawing import RenderContext, Frontend
from ezdxf.addons.drawing.matplotlib import MatplotlibBackend
import matplotlib.pyplot as plt

def convert_dxf_to_png(dxf_path, out_png_path, dpi=200):
    try:
        doc = ezdxf.readfile(dxf_path)
    except Exception as e:
        return {"success": False, "error": f"Failed to parse DXF: {str(e)}", "code": "CORRUPTED_DXF"}

    msp = doc.modelspace()

    # Extract metadata: units, extents, layer names
    units_code = doc.header.get("$INSUNITS", 0)
    units_map = {
        0: "Unspecified",
        1: "Inches",
        2: "Feet",
        3: "Miles",
        4: "Millimeters",
        5: "Centimeters",
        6: "Meters",
        7: "Kilometers",
    }
    unit_str = units_map.get(units_code, "Unspecified")
    layers = [layer.dxf.name for layer in doc.layers]

    # Calculate bounding extents if available
    extents = []
    try:
        extmin = doc.header.get("$EXTMIN", (0, 0, 0))
        extmax = doc.header.get("$EXTMAX", (100, 100, 0))
        extents = [float(extmin[0]), float(extmin[1]), float(extmax[0]), float(extmax[1])]
    except:
        extents = [0, 0, 100, 100]

    # Render drawing using ezdxf matplotlib backend
    try:
        fig = plt.figure(figsize=(12, 12))
        ax = fig.add_axes([0, 0, 1, 1])
        ax.set_aspect("equal", "datalim")
        ax.axis("off")

        ctx = RenderContext(doc)
        out = MatplotlibBackend(ax)
        frontend = Frontend(ctx, out)
        frontend.draw_layout(msp, finalize=True)

        out_dir = os.path.dirname(out_png_path)
        if out_dir:
            os.makedirs(out_dir, exist_ok=True)
        fig.savefig(out_png_path, dpi=dpi, bbox_inches="tight", pad_inches=0.1, facecolor="white")
        plt.close(fig)

        # Get final rendered image dimensions
        from PIL import Image
        with Image.open(out_png_path) as img:
            rendered_w, rendered_h = img.size

        return {
            "success": True,
            "filename": os.path.basename(out_png_path),
            "width": rendered_w,
            "height": rendered_h,
            "dpi": dpi,
            "units": unit_str,
            "extents": extents,
            "layers": layers,
            "dxf_version": doc.dxfversion
        }
    except Exception as e:
        plt.close('all')
        return {"success": False, "error": f"Rendering DXF failed: {str(e)}", "code": "RENDER_FAILED"}

def main():
    if len(sys.argv) < 3:
        print(json.dumps({"success": False, "error": "Missing arguments"}))
        sys.exit(1)

    cmd = sys.argv[1]
    if cmd == "convert":
        dxf_path = sys.argv[2]
        out_png = sys.argv[3]
        dpi = int(sys.argv[4]) if len(sys.argv) > 4 else 200
        res = convert_dxf_to_png(dxf_path, out_png, dpi)
        print(json.dumps(res))
    else:
        print(json.dumps({"success": False, "error": f"Unknown command: {cmd}"}))
        sys.exit(1)

if __name__ == "__main__":
    main()
