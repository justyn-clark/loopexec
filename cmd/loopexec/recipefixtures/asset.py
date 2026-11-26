import bpy,sys,json,os,math
from mathutils import Vector
args=sys.argv[sys.argv.index("--")+1:]
mode=args[0]; context=json.load(open(args[1]));root=context["root"];i=context["inputs"]
source=os.path.join(root,i["source"]);output=os.path.join(root,i["output"])
if mode=="create":
    bpy.ops.wm.read_factory_settings(use_empty=True)
    bpy.ops.mesh.primitive_cube_add()
    obj=bpy.context.object
    sub=obj.modifiers.new("Dense authored surface","SUBSURF");sub.subdivision_type="SIMPLE";sub.levels=4
    bpy.ops.object.modifier_apply(modifier=sub.name)
    mat=bpy.data.materials.new("JC blue");mat.diffuse_color=(0.1,0.5,0.8,1);obj.data.materials.append(mat)
    bpy.ops.wm.save_as_mainfile(filepath=source)
    bpy.ops.wm.save_as_mainfile(filepath=output)
elif mode=="optimize":
    bpy.ops.wm.open_mainfile(filepath=source)
    obj=next(o for o in bpy.context.scene.objects if o.type=="MESH")
    bpy.context.view_layer.objects.active=obj;obj.select_set(True)
    mod=obj.modifiers.new("Dissolve coplanar geometry","DECIMATE");mod.decimate_type="DISSOLVE";mod.angle_limit=0.01
    bpy.ops.object.modifier_apply(modifier=mod.name)
    bpy.ops.wm.save_as_mainfile(filepath=output)
elif mode=="measure":
    def measure(path,label):
        bpy.ops.wm.open_mainfile(filepath=path)
        objects=[o for o in bpy.context.scene.objects if o.type=="MESH"]
        triangles=sum(sum(len(poly.vertices)-2 for poly in o.data.polygons) for o in objects)
        texture=max([max(im.size) for im in bpy.data.images if im.source=="FILE"]+[0])
        checks={"uvs":all(len(o.data.uv_layers)>0 for o in objects),"materials":all(len(o.data.materials)>0 for o in objects),"normals":all(all(v.normal.length>0.9 for v in o.data.vertices) for o in objects),"scale":all(all(abs(v-1)<1e-5 for v in o.scale) for o in objects),"import":len(objects)>0}
        masks={}
        scene=bpy.context.scene;scene.render.engine="BLENDER_EEVEE";scene.render.resolution_x=128;scene.render.resolution_y=128;scene.render.resolution_percentage=100;scene.render.film_transparent=True
        for view in i["views"]:
            if view not in ["front","side","three-quarter"]:raise ValueError("unsupported fixture view")
            pos={"front":(0,-6,0),"side":(6,0,0),"three-quarter":(4,-6,3)}[view]
            data=bpy.data.cameras.new("comparison");camera=bpy.data.objects.new("comparison",data);scene.collection.objects.link(camera);camera.location=pos;camera.rotation_euler=(-camera.location).to_track_quat("-Z","Y").to_euler();data.type="ORTHO";data.ortho_scale=4;scene.camera=camera
            scene.render.filepath=os.path.join(context["evidence_dir"],label+"-"+view+".png")
            bpy.ops.render.render(write_still=True)
            image=bpy.data.images.load(scene.render.filepath,check_existing=False)
            masks[view]=[a>0.5 for a in list(image.pixels)[3::4]]
        return triangles,texture,checks,masks
    a=measure(source,"source");b=measure(output,"candidate")
    metrics={"triangles":b[0],"texture_edge":b[1]}
    checks=b[2]
    for view in i["views"]:
        union=sum(x or y for x,y in zip(a[3][view],b[3][view]));intersection=sum(x and y for x,y in zip(a[3][view],b[3][view]));metrics["similarity:"+view]=intersection/union if union else 0;checks["view:"+view]=union>0
    print("RECIPE_REPORT:"+json.dumps({"checks":checks,"metrics":metrics}))
