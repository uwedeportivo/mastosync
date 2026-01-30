(* code enhanced from https://mathematica.stackexchange.com/q/136974 *)

argv = Rest @ $ScriptCommandLine;
argc = Length @ argv;

mandalaPath = If[argc >= 1, argv[[1]], "/tmp"];

(* Enhanced Color Management *)
themes = {
  "BrightBands", "IslandColors", "FruitPunchColors", 
  "AvocadoColors", "Rainbow", "SouthwestColors", "Aquamarine"
};

randomColor[opacity_ : 1.0] := 
  Directive[
    ColorData[RandomChoice[themes]][RandomReal[]], 
    Opacity[opacity]
  ];

(* Existing shapes with enhancements *)
flower[n_, a_, r_] := {
  FaceForm[randomColor[0.8]], 
  EdgeForm[{randomColor[1.0], Thickness[0.002]}],
  Module[{b = RandomChoice[{-1/(2 n), 0, 1/(2 n)}]}, 
   Cases[
    ParametricPlot[
     r (a + Cos[n t])/(a + 1) {Cos[t + b Sin[2 n t]], Sin[t + b Sin[2 n t]]}, 
     {t, 0, 2 Pi}, PlotPoints -> 100], 
    l_Line :> FilledCurve[l], -1]]
}

disk[n_, a_, r_] := {
  FaceForm[randomColor[0.7]], 
  EdgeForm[{randomColor[1.0], Thickness[0.003]}],
  Disk[{0, 0}, r]
}

spots[n_, a_, r_] := {
  FaceForm[randomColor[0.9]], 
  EdgeForm[None],
  Translate[Disk[{0, 0}, r a/(6 n)], r CirclePoints[n]]
}

(* New Shape: Rings *)
rings[n_, a_, r_] := {
  FaceForm[randomColor[0.6]],
  EdgeForm[{randomColor[0.8], Thickness[0.001]}],
  Annulus[{0, 0}, {r * 0.8, r}]
}

(* New Shape: Stars *)
stars[n_, a_, r_] := {
  FaceForm[randomColor[0.85]],
  EdgeForm[{Black, Thickness[0.001]}],
  Polygon[r Riffle[CirclePoints[n], CirclePoints[n]*0.5]]
}

(* Enhanced Mandala Composition *)
mandala[n_, layers_] := 
 Graphics[{
   EdgeForm[Black], White, 
   Table[
    With[{
      shapeFunc = RandomChoice[{4, 2, 2, 1, 1} -> {flower, disk, rings, spots, stars}],
      rot = RandomChoice[{0, Pi/n, Pi/(2n)}]
    },
      shapeFunc[n, RandomReal[{3, 7}], i] ~Rotate~ (rot + RandomReal[{-0.1, 0.1}])
    ], 
    {i, layers, 1, -1}
   ]
  }, 
  PlotRange -> All, 
  Background -> None
 ]

exportPath = FileNameJoin[{mandalaPath, "mandala.png"}];
Print["Generating mandala to: ", exportPath];
Export[exportPath, mandala[16, 25], "PNG", Background -> None];
