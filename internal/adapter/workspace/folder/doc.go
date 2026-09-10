// Package folder provides direct filesystem snapshots, persistent recovery copies
// and cooperative locks for the selected folder. It never invokes Git or inspects
// commits, indexes or repository status. VCS metadata and ignored files are outside
// the evidence boundary. Symlinks are recorded without following their targets.
package folder
