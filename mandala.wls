(* code from https://mathematica.stackexchange.com/q/136974 *)

argv = Rest @ $ScriptCommandLine;
argc = Length @ argv;

mandalaPath = If[argc >=1, argv[[1]], "/tmp"];

randomColor[] :=
  ColorData[
    RandomChoice[{"BrightBands", "IslandColors", "FruitPunchColors", 
      "AvocadoColors", "Rainbow"}]][RandomReal[]];

flower[n_, a_, r_] := {FaceForm[randomColor[]], 
  Module[{b = RandomChoice[{-1/(2 n), 0}]}, 
   Cases[
    ParametricPlot[
     r (a + Cos[n t])/(a + 1) {Cos[t + b Sin[2 n t]], 
       Sin[t + b Sin[2 n t]]}, {t, 0, 2 Pi}], 
    l_Line :> FilledCurve[l], -1]]}

disk[_, _, r_] := {FaceForm[randomColor[]], Disk[{0, 0}, r]}

spots[n_, a_, r_] := {FaceForm[randomColor[]], 
  Translate[Disk[{0, 0}, r a/(4 n)], r CirclePoints[n]]}

mandala[n_, m_] := 
 Graphics[{EdgeForm[Black], White, 
   Table[
    RandomChoice[{3, 2, 1} -> {flower, disk, spots}][n, 
      RandomReal[{3, 6}], i]~Rotate~(Pi i/n), {i, m, 1, -1}]}, 
  PlotRange -> All]

Export[FileNameJoin[{mandalaPath, "mandala.png"}], RemoveBackground @ mandala[16, 20]];
