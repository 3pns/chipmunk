package chipmunk

import (
	"fmt"
	"github.com/vova616/chipmunk/transform"
	"github.com/vova616/chipmunk/vect"
	
	"time"
)

// Used to keep a linked list of all arbiters on a body.
type ArbiterEdge struct {
	Arbiter    *Arbiter
	Next, Prev *ArbiterEdge
	Other      *Body
}

type arbiterState int

const (
	arbiterStateFirstColl = iota
	arbiterStateNormal
)

// The maximum number of ContactPoints a single Arbiter can have.
const MaxPoints = 4

type Arbiter struct {
	// The two colliding shapes.
	ShapeA, ShapeB *Shape
	// The contact points between the shapes.
	Contacts *[MaxPoints]*Contact
	// The number of contact points.
	NumContacts int

	nodeA, nodeB *ArbiterEdge

	/// Calculated value to use for the elasticity coefficient.
	/// Override in a pre-solve collision handler for custom behavior.
	e vect.Float
	/// Calculated value to use for the friction coefficient.
	/// Override in a pre-solve collision handler for custom behavior.
	u vect.Float
	 /// Calculated value to use for applying surface velocities.
	/// Override in a pre-solve collision handler for custom behavior.
	Surface_vr vect.Vect

	state arbiterState
	stamp time.Duration
}

func newArbiter() *Arbiter {
	return new(Arbiter)
}



func (arb *Arbiter) destroy() {
	arb.ShapeA = nil
	arb.ShapeB = nil
	arb.NumContacts = 0
	arb.u = 0
	arb.e = 0
}

func (arb1 *Arbiter) equals(arb2 *Arbiter) bool {
	if arb1.ShapeA == arb2.ShapeA && arb1.ShapeB == arb2.ShapeB {
		return true
	}

	return false
}

func (arb *Arbiter) update(contacts *[MaxPoints]*Contact, numContacts int) {
	oldContacts := arb.Contacts
	oldNumContacts := arb.NumContacts

	sa := arb.ShapeA
	sb := arb.ShapeB

	for i := 0; i < oldNumContacts; i++ {
		oldC := oldContacts[i]
		for j := 0; j < numContacts; j++ {
			newC := contacts[j]

			if newC.hash == oldC.hash {
				newC.jnAcc = oldC.jnAcc
				newC.jtAcc = oldC.jtAcc
				newC.jBias = oldC.jBias
			}
		}
	}

	arb.Contacts = contacts
	arb.NumContacts = numContacts

	arb.u = sa.u * sb.u
	arb.e = sa.e * sb.e

	arb.Surface_vr = vect.Sub(sa.Surface_v, sb.Surface_v)
}

func (arb *Arbiter) preStep(inv_dt, slop, bias vect.Float) {

	a := arb.ShapeA.Body
	b := arb.ShapeB.Body

	for i,con := range arb.Contacts {
		if i >= arb.NumContacts {
			break
		}

		// Calculate the offsets.
		con.r1 = vect.Sub(con.p, a.p)
		con.r2 = vect.Sub(con.p, b.p)
 
		//con.Normal = vect.Vect{-1,0}
		

		// Calculate the mass normal and mass tangent.
		n := con.n
		rcn := (con.r1.X*n.Y) - (con.r1.Y*n.X)
		rcn = a.m_inv + (a.i_inv*rcn*rcn)
		
		rcn2 := (con.r2.X*n.Y) - (con.r2.Y*n.X)
		rcn2 = b.m_inv + (b.i_inv*rcn2*rcn2)
		
		value := rcn + rcn2
		if value == 0.0 {
			fmt.Printf("Warning: Unsolvable collision or constraint.")
		}
		con.nMass = 1.0 / value
		
		n = vect.Perp(con.n)
		rcn = (con.r1.X*n.Y) - (con.r1.Y*n.X)
		rcn = a.m_inv + (a.i_inv*rcn*rcn)
		
		rcn2 = (con.r2.X*n.Y) - (con.r2.Y*n.X)
		rcn2 = b.m_inv + (b.i_inv*rcn2*rcn2)
		
		value = rcn + rcn2
		if value == 0.0 {
			fmt.Printf("Warning: Unsolvable collision or constraint.")
		}
		con.tMass = 1.0 / value

		// Calculate the target bias velocity.
		ds := con.dist+slop
		if 0 > ds {
			con.bias = -bias * inv_dt * con.dist+slop
		} else {
			con.bias = 0;
		}
		con.jBias = 0.0
		con.bounce = vect.Dot(vect.Vect{(-con.r2.Y*b.w+b.v.X)-(-con.r1.Y*a.w+a.v.X), (con.r2.X*b.w+b.v.Y)-(con.r1.X*a.w+a.v.Y)}, con.n) * arb.e

	}
}

func (arb *Arbiter) preStep2(inv_dt, slop, bias vect.Float) {

	a := arb.ShapeA.Body
	b := arb.ShapeB.Body

	for i := 0; i < arb.NumContacts; i++ {
		con := arb.Contacts[i]

		// Calculate the offsets.
		con.r1 = vect.Sub(con.p, a.p)
		con.r2 = vect.Sub(con.p, b.p)

		//con.Normal = vect.Vect{-1,0}
		

		// Calculate the mass normal and mass tangent.
		con.nMass = 1.0 / k_scalar(a, b, con.r1, con.r2, con.n)
		con.tMass = 1.0 / k_scalar(a, b, con.r1, con.r2, vect.Perp(con.n))

		// Calculate the target bias velocity.
		con.bias = -bias * inv_dt * vect.FMin(0.0, con.dist+slop)
		con.jBias = 0.0
		//con.jtAcc = 0
		//con.jnAcc = 0
		//fmt.Println("con.dist", con.dist)
		
		// Calculate the target bounce velocity.
		con.bounce = normal_relative_velocity(a, b, con.r1, con.r2, con.n) * arb.e
	}
}

//Optimized applyCachedImpulse
func (arb *Arbiter) applyCachedImpulse(dt_coef vect.Float) {
	if arb.state == arbiterStateFirstColl && arb.NumContacts > 0 {
		return
	}
	//println("asd")
	a := arb.ShapeA.Body
	b := arb.ShapeB.Body
	var j vect.Vect
	
	for i,con := range arb.Contacts {
		if i >= arb.NumContacts {
			break
		}
	
		j.X = ((con.n.X*con.jnAcc) - (con.n.Y*con.jtAcc)) * dt_coef
		j.Y = ((con.n.X*con.jtAcc) + (con.n.Y*con.jnAcc)) * dt_coef
		
		a.v.X = (-j.X*a.m_inv)+a.v.X
		a.v.Y = (-j.Y*a.m_inv)+a.v.Y
		a.w += a.i_inv * ((con.r1.X*-j.Y) - (con.r1.Y*-j.X))
		
		b.v.X = (j.X*b.m_inv)+b.v.X
		b.v.Y = (j.Y*b.m_inv)+b.v.Y
	
		b.w += b.i_inv * ((con.r2.X*j.Y) - (con.r2.Y*j.X))
	}
} 

func (arb *Arbiter) applyCachedImpulse2(dt_coef vect.Float) {
	if arb.state == arbiterStateFirstColl && arb.NumContacts > 0 {
		return
	}
	//println("asd")
	a := arb.ShapeA.Body
	b := arb.ShapeB.Body
	for i,con := range arb.Contacts {
		if i >= arb.NumContacts {
			break
		}
		j := transform.RotateVect(con.n, transform.Rotation{con.jnAcc, con.jtAcc})
		apply_impulses(a, b, con.r1, con.r2, vect.Mult(j, dt_coef))
	}
} 
 
/*
func (arb *Arbiter) applyImpulse() {
	a := arb.ShapeA.Body
	b := arb.ShapeB.Body   

	for i := 0; i < arb.NumContacts; i++ {
		con := arb.Contacts[i]
		Impulse(a,b,con,arb.Surface_vr,float32(arb.u))
	}
}
*/

//Optimized applyImpulse
func (arb *Arbiter) applyImpulse() {
	a := arb.ShapeA.Body
	b := arb.ShapeB.Body   
	vr := vect.Vect{}

	for i,con := range arb.Contacts {
		if i >= arb.NumContacts {
			break
		}
		n := con.n
		r1 := con.r1
		r2 := con.r2
		
		vr.X = (-r2.Y*b.w+b.v.X)-(-r1.Y*a.w+a.v.X)
		vr.Y = (r2.X*b.w+b.v.Y)-(r1.X*a.w+a.v.Y)

		// Calculate and clamp the bias impulse.
		jbnOld := con.jBias
		con.jBias = jbnOld+(con.bias - (((((-r2.Y*b.w_bias)+b.v_bias.X)-((-r1.Y*a.w_bias)+a.v_bias.X))*n.X) + ((((r2.X*b.w_bias)+b.v_bias.Y)-((r1.X*a.w_bias)+a.v_bias.Y))*n.Y))) * con.nMass
		if 0 > con.jBias {
			con.jBias = 0
		}
		
		// Calculate and clamp the normal impulse.
		jnOld := con.jnAcc
		con.jnAcc = jnOld-(con.bounce + (vr.X*n.X) + (vr.Y*n.Y)) * con.nMass
		if 0 > con.jnAcc {
			con.jnAcc = 0
		}


		// Calculate and clamp the friction impulse.
		jtMax := arb.u * con.jnAcc
		jtOld := con.jtAcc
		con.jtAcc = jtOld-(((vr.X+arb.Surface_vr.X)*-n.Y) + ((vr.Y+arb.Surface_vr.Y)*n.X)) * con.tMass
		if con.jtAcc > jtMax {
			con.jtAcc = jtMax
		} else if con.jtAcc < -jtMax {
			con.jtAcc = -jtMax
		}
 

		jbnOld = (con.jBias-jbnOld)
		vr.X = n.X*jbnOld
		vr.Y = n.Y*jbnOld
		
		a.v_bias.X = (-vr.X*a.m_inv)+a.v_bias.X
		a.v_bias.Y = (-vr.Y*a.m_inv)+a.v_bias.Y
		a.w_bias += a.i_inv * ((r1.X*-vr.Y) - (r1.Y*-vr.X))

		b.v_bias.X = (vr.X*b.m_inv)+b.v_bias.X
		b.v_bias.Y = (vr.Y*b.m_inv)+b.v_bias.Y
		b.w_bias += b.i_inv * ((r2.X*vr.Y) - (r2.Y*vr.X))


		jnOld = con.jnAcc - jnOld
		jtOld = con.jtAcc - jtOld

		vr.X = (n.X*jnOld) - (n.Y*jtOld)
		vr.Y = (n.X*jtOld) + (n.Y*jnOld)		
		 
		a.v.X = (-vr.X*a.m_inv)+a.v.X
		a.v.Y = (-vr.Y*a.m_inv)+a.v.Y
		a.w += a.i_inv * ((r1.X*-vr.Y) - (r1.Y*-vr.X))
		
		b.v.X = (vr.X*b.m_inv)+b.v.X
		b.v.Y = (vr.Y*b.m_inv)+b.v.Y

		b.w += b.i_inv * ((r2.X*vr.Y) - (r2.Y*vr.X))
	}
}


func (arb *Arbiter) applyImpulse2() {
	a := arb.ShapeA.Body
	b := arb.ShapeB.Body

	for i := 0; i < arb.NumContacts; i++ {
		con := arb.Contacts[i]
		n := con.n
		r1 := con.r1
		r2 := con.r2

		// Calculate the relative bias velocities.
		vb1 := vect.Add(a.v_bias, vect.Mult(vect.Perp(r1), a.w_bias))
		vb2 := vect.Add(b.v_bias, vect.Mult(vect.Perp(r2), b.w_bias))
		vbn := vect.Dot(vect.Sub(vb2, vb1), n)

		

		// Calculate the relative velocity.
		vr := relative_velocity(a, b, r1, r2)
		vrn := vect.Dot(vr, n)
		// Calculate the relative tangent velocity.
		vrt := vect.Dot(vect.Add(vr, arb.Surface_vr), vect.Perp(n))

		// Calculate and clamp the bias impulse.
		jbn := (con.bias - vbn) * con.nMass
		jbnOld := con.jBias
		con.jBias = vect.FMax(jbnOld+jbn, 0.0)
		
		
		// Calculate and clamp the normal impulse.
		jn := -(con.bounce + vrn) * con.nMass
		jnOld := con.jnAcc
		con.jnAcc = vect.FMax(jnOld+jn, 0.0)


		// Calculate and clamp the friction impulse.
		jtMax := arb.u * con.jnAcc
		jt := -vrt * con.tMass
		jtOld := con.jtAcc
		con.jtAcc = vect.FClamp(jtOld+jt, -jtMax, jtMax)


		// Apply the bias impulse.
		apply_bias_impulses(a, b, r1, r2, vect.Mult(n, con.jBias-jbnOld))

		// Apply the final impulse.
		apply_impulses(a, b, r1, r2, transform.RotateVect(n, transform.Rotation{con.jnAcc - jnOld, con.jtAcc - jtOld}))
		 
	}
}

